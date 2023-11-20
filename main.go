package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
	openai "github.com/sashabaranov/go-openai"
	"github.com/spf13/viper"
)

var MONTH_LIMIT = viperEnvVariable("MONTH_LIMIT")

type MessageEntry struct {
	From	string
	FromName	string
	To      string
	ToName string
	Profession string
	Company string
	Date    time.Time
	isSent bool
}

type DatabaseEntry struct {
	Name    string
	Email      string
	SentEmails int
	ReceivedEmails int
	LastDate    time.Time
	Profession    string
	Company    string
	// Date    time.Time
}

func viperEnvVariable(key string) string {
	viper.SetConfigFile(".env")
	err := viper.ReadInConfig()
  
	if err != nil {
	  log.Fatalf("Error while reading config file %s", err)
	}
	value, ok := viper.Get(key).(string)
	if !ok {
	  log.Fatalf("Invalid type assertion")
	}
	return value
  }

func main() {
    // gin.SetMode(gin.ReleaseMode)
    // router := gin.New()
    // router.GET("/processEmail", fetchEmailsHandler)

	// log.Println("Listening and serving HTTP on localhost:8080")
    // router.Run("localhost:8080")
	fetchEmailsHandler()
}

// func fetchEmailsHandler(_c *gin.Context) {
func fetchEmailsHandler() {
	imapserver := "outlook.office365.com:993"
	var useremail string
	var password string
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("Enter your email address: ")
	scanner.Scan()
	useremail = scanner.Text()

	fmt.Print("Enter your password: ")
	scanner.Scan()
	password = scanner.Text()

	// useremail := "miratest123@outlook.com"
	// password := "Mira!Test!123"
	
	log.Println("Processing emails...")

	log.Println("Connecting to server...")

	c, err := imapclient.DialTLS(imapserver, nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Connected")

	defer c.Close()

	if err := c.Login(useremail, password).Wait(); err != nil {
		log.Fatalf("failed to login: %v", err)
	}
	log.Println("Logged in")

	var messageEntries []MessageEntry
	var databaseEntries []DatabaseEntry
	receivedMessageEntries := processEmails(c, "INBOX", false)
	sentMessageEntries := processEmails(c, "Sent", true)
	messageEntries = append(messageEntries, receivedMessageEntries...)
	messageEntries = append(messageEntries, sentMessageEntries...)

	for _, entry := range messageEntries {
		if(entry.isSent) {
			entry.From = useremail
		}
		email := ""
		if(entry.isSent) {
			email = entry.To
		} else {
			email = entry.From
		}
		if !emailExistsInDatabase(email, databaseEntries) {
			sentEmails := 0
			receivedEmails := 0
			name := ""
			if entry.isSent {
				name = entry.ToName
				sentEmails++
			} else {
				name = entry.FromName
				receivedEmails++
			}

			databaseEntry := DatabaseEntry{
				Name:            name,
				Email:           email,
				SentEmails:	     sentEmails,
				ReceivedEmails:	 receivedEmails,
				Profession:		 entry.Profession,
				Company:		 entry.Company,
				LastDate:		 entry.Date,
			}
			databaseEntries = append(databaseEntries, databaseEntry)
		} else {
			for i := range databaseEntries {
				if databaseEntries[i].Email == email {
					if entry.To == useremail {
						databaseEntries[i].ReceivedEmails++
						if databaseEntries[i].Name == "" {
							databaseEntries[i].Name = entry.FromName
						}
					} else if entry.From == useremail{
						databaseEntries[i].SentEmails++
					} else{
						databaseEntries[i].ReceivedEmails++
					}
	
					if entry.Date.After(databaseEntries[i].LastDate) {
						databaseEntries[i].LastDate = entry.Date
					}
				}
			}
		}
	}

	if err := exportToCSV(databaseEntries); err != nil {
		log.Fatal(err)
	}

	
	log.Println("Done!")
}

func processEmails(c *imapclient.Client, mailboxName string, isSent bool) []MessageEntry {
	var messageEntries []MessageEntry
	log.Println("Fetching " + mailboxName + " messages...")
	mbox, err := c.Select(mailboxName, nil).Wait()
	if err != nil {
		log.Fatalf("failed to select INBOX: %v", err)
	}
	
	// Get the whole message body
	seqSet := imap.SeqSetRange(1, mbox.NumMessages)
	fetchOptions := &imap.FetchOptions{
		UID: true,
		Flags:    true,
		Envelope: true,
		BodySection: []*imap.FetchItemBodySection{{}},
	}
	messages := c.Fetch(seqSet, fetchOptions)
	for {
		msg := messages.Next()
		if msg == nil {
			break
		}
		content := ""
		date := time.Now()
		from := make([]imap.Address, 0)
		to := make([]imap.Address, 0)
		for {
			item := msg.Next()
			if item == nil {
				break
			}
			
			switch item := item.(type) {
			case imapclient.FetchItemDataEnvelope:
				date = item.Envelope.Date
				if err != nil {
					log.Fatal(err)
				}
				difference := time.Now().Sub(date)
				months := difference.Hours() / (24 * 365.25 / 12)
				month_limit, err := strconv.ParseFloat(MONTH_LIMIT, 64)
				if err != nil {
					log.Fatal("Error:", err)
				}
				if months >= month_limit {
					break
				}

				from = item.Envelope.From
				if err != nil {
					log.Fatal(err)
				}
				to = item.Envelope.To
				if err != nil {
					log.Fatal(err)
				}
			case imapclient.FetchItemDataBodySection:
				mr, err := mail.CreateReader(item.Literal)
				if err != nil {
					log.Fatal(err)
				}
				for {
					p, err := mr.NextPart()
					if err == io.EOF {
						break
					} else if err != nil {
						log.Fatal(err)
					}
			
					switch h := p.Header.(type) {
					case *mail.InlineHeader:
						contentType, _, err := h.ContentType()
						b, _ := io.ReadAll(p.Body)
						if err != nil {
							log.Fatal(err)
						}
						if contentType == "text/plain" {
							content = string(b)
						} else if contentType == "text/html" {
							if content == "" {
								content = extractContent(string(b))
						}
						}
					case *mail.AttachmentHeader:
						filename, _ := h.Filename()
						log.Printf("Got attachment: %v\n", filename)
					}
				}
			}
		}
		profession, company := extractInformation(content)

		entry := MessageEntry{
			Date:    	 date,
			From:    	 from[0].Mailbox + "@" + from[0].Host,
			FromName:    from[0].Name,
			To:      	 to[0].Mailbox + "@" + to[0].Host,
			ToName:      to[0].Name,
			Profession:  profession,
			Company:   company,
			isSent:   	 isSent,
		}
		messageEntries = append(messageEntries, entry)
	}
	return messageEntries
}

func extractContent(input string) string {
	htmlPattern := `(?s)<html[^>]*>.*?</html>`
	htmlReg := regexp.MustCompile(htmlPattern)
	htmlContent := htmlReg.FindString(input)

	bodyPattern := `(?s)<body[^>]*>.*?</body>`
	bodyReg := regexp.MustCompile(bodyPattern)
	bodyContent := bodyReg.FindString(htmlContent)

	textContent := strings.Join([]string{
		strings.TrimSpace(stripTags(bodyContent)),
	}, " ")

	if textContent != "" {
		return applyPatterns(textContent)
	}

	contentPattern := `(?s)Content-Type: text/plain;.*?Content-Type: text/html`
	re := regexp.MustCompile(contentPattern)
	match := re.FindString(input)

	return applyPatterns(match)
}

func stripTags(input string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(input, "")
}

func applyPatterns(input string) string {
	newlinePattern := `\n`
	removePattern := `=\S*`
	ampersandPattern := `&[^;]*;`
	spacesPattern := `\s{2,}`

	newlineReg := regexp.MustCompile(newlinePattern)
	removeReg := regexp.MustCompile(removePattern)
	ampersandReg := regexp.MustCompile(ampersandPattern)
	spacesReg := regexp.MustCompile(spacesPattern)

	output := newlineReg.ReplaceAllString(input, "")
	output = removeReg.ReplaceAllString(output, "")
	output = ampersandReg.ReplaceAllString(output, "")
	output = spacesReg.ReplaceAllString(output, " ")

	return output
}

func emailExistsInDatabase(email string, entries []DatabaseEntry) bool {
	for _, obj := range entries {
        if obj.Email == email {
            return true
        }
    }
    return false
}

func extractInformation(content string) (string, string) {
	log.Println("==============> content: ", content)
	apiKey := "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"
	ctx := context.Background()
	client := openai.NewClient(apiKey)
	prompt := `A person by the name of [x] has an email of [y], in one word each, what company do they work at (spell it out fully) and what is their likely profession?
			   You sent them [x] emails and they responded with [y] email. Categorize the relationship. 
			   In your response, only respond with "engaged," "sometimes engaged" or "unresponsive" in your response`
	inputText := prompt + content
	professionRegex := regexp.MustCompile(`Profession:\s*([^\n]+)`)
	companyRegex := regexp.MustCompile(`Company:\s*([^\n]+)`)

	response, err := client.CreateChatCompletion(ctx,
		openai.ChatCompletionRequest{
			Model: openai.GPT4,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: inputText,
				},
			},
			MaxTokens: 150,
		},
	)
	if err != nil {
		log.Fatal("Error generating response from OpenAI:", err)
	}
	
	extractedInfo := response.Choices[0].Message.Content
	log.Println("===========> extractedInfo: ", extractedInfo)

	professionMatches := professionRegex.FindStringSubmatch(extractedInfo)
	companyMatches := companyRegex.FindStringSubmatch(extractedInfo)
	var profession, company string

	if len(professionMatches) > 1 {
		profession = strings.TrimSpace(professionMatches[1])
	}

	if len(companyMatches) > 1 {
		company = strings.TrimSpace(companyMatches[1])
	}

	return profession, company
}

func exportToCSV(entries []DatabaseEntry) error {
	file, err := os.Create("messages.csv")
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"Name", "Email", "SentEmails", "ReceivedEmails", "Profession", "Company", "LastDate"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, entry := range entries {
		row := []string{
			entry.Name,
			entry.Email,
			strconv.Itoa(entry.SentEmails),
			strconv.Itoa(entry.ReceivedEmails),
			entry.Profession,
			entry.Company,
			entry.LastDate.Format("2006-01-02 15:04:05"),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}