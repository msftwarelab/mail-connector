package main

import (
	"encoding/csv"
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
	"github.com/spf13/viper"
)

var MONTH_LIMIT = viperEnvVariable("MONTH_LIMIT")

type MessageEntry struct {
	From			 string
	FromName		 string
	To      		 string
	ToName 			 string
	Profession 		 string
	Company 		 string
	Date    	 	 time.Time
	MeetingDate 	 time.Time
	MeetingInfo 	 string
	Cc 			 	 []imap.Address
	isSent 			 bool
	isMeeting 		 bool
	isVirtualMeeting bool
}

type DatabaseEntry struct {
	Name    	      	 string
	Email      		  	 string
	SentEmails 		  	 int
	ReceivedEmails 	  	 int
	LastDate    	  	 time.Time
	Profession    	  	 string
	Company    		  	 string
	TotalMeetings 	  	 int
	Cc 				  	 string
	VirtualMeetings   	 int
	PhysicalMeetings  	 int
	FrequencyMeetings  	 int
	FrequencyOfMeetings  float32
	Relationship      	 string
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
	// var useremail string
	// var password string
	// scanner := bufio.NewScanner(os.Stdin)

	// fmt.Print("Enter your email address: ")
	// scanner.Scan()
	// useremail = scanner.Text()

	// fmt.Print("Enter your password: ")
	// scanner.Scan()
	// password = scanner.Text()

	useremail := "miratest123@outlook.com"
	password := "Mira!Test!123"

	// useremail := "acetopcloud@outlook.com"
	// password := "cloudeast3k"
	
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
	receivedMessageEntries := processEmails(c, "INBOX", false, "inbox.txt")
	sentMessageEntries := processEmails(c, "Sent", true, "sent.txt")
	messageEntries = append(messageEntries, receivedMessageEntries...)
	messageEntries = append(messageEntries, sentMessageEntries...)

	log.Println("Extracting out spam messages...")
	// Extracting out spam messages..

	for _, entry := range messageEntries {
		Cc := ""
		for i := range entry.Cc {
			Cc += entry.Cc[i].Mailbox + "@" + entry.Cc[i].Host + "; "
		}
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
			totalMeetings := 0
			virtualMeetings := 0
			physicalMeetings := 0
			name := ""
			frequencyMeetings := 0
			daysBetween := int(time.Now().Sub(entry.MeetingDate) / 24 / time.Hour)
			if daysBetween < 60 {
				frequencyMeetings = 1
			}
			frequencyOfMeetings := float32(frequencyMeetings) / 60
			if entry.isMeeting {
				totalMeetings = 1
				if entry.isVirtualMeeting {
					virtualMeetings = 1
				} else {
					physicalMeetings = 1
				}
			}
			if entry.isSent {
				name = entry.ToName
				sentEmails++
			} else {
				name = entry.FromName
				receivedEmails++
			}

			databaseEntry := DatabaseEntry{
				Name:              name,
				Email:             email,
				SentEmails:	       sentEmails,
				ReceivedEmails:	   receivedEmails,
				LastDate:		   entry.Date,
				TotalMeetings:     totalMeetings,
				Cc:                Cc,
				VirtualMeetings:   virtualMeetings,
				PhysicalMeetings:  physicalMeetings,
				FrequencyMeetings:  frequencyMeetings,
				FrequencyOfMeetings:  float32(frequencyOfMeetings),
			}
			databaseEntries = append(databaseEntries, databaseEntry)
		} else {
			for i := range databaseEntries {
				if databaseEntries[i].Email == email {
					daysBetween := int(time.Now().Sub(entry.MeetingDate) / 24 / time.Hour)
					if daysBetween < 60 {
						databaseEntries[i].FrequencyMeetings++
						databaseEntries[i].FrequencyOfMeetings = float32(databaseEntries[i].FrequencyMeetings) / 60
					}
					for i := range entry.Cc {
						databaseEntries[i].Cc += entry.Cc[i].Mailbox + "@" + entry.Cc[i].Host + "; "
					}
					if entry.isMeeting {
						databaseEntries[i].TotalMeetings++
						if entry.isVirtualMeeting {
							databaseEntries[i].VirtualMeetings++
						} else {
							databaseEntries[i].PhysicalMeetings++
						}
					}
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

	log.Println("Extracting profesion and company...")

	for i := range databaseEntries {
		profession, company, relationship := extractInformation(databaseEntries[i].Name, databaseEntries[i].Email, databaseEntries[i].SentEmails, databaseEntries[i].ReceivedEmails)
		databaseEntries[i].Profession = profession
		databaseEntries[i].Company = company
		databaseEntries[i].Relationship = relationship
	}

	if err := exportToCSV(databaseEntries); err != nil {
		log.Fatal(err)
	}

	log.Println("Done!")
}

func processEmails(c *imapclient.Client, mailboxName string, isSent bool, fileName string) []MessageEntry {
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

	file, err := os.Create(fileName)
	if err != nil {
		log.Fatal("Error creating file: ", err)
	}
	defer file.Close()
	
	messages := c.Fetch(seqSet, fetchOptions)
	for {
		msg := messages.Next()
		if msg == nil {
			break
		}
		content := ""
		isMeeting := false
		isVirtualMeeting := false
		date := time.Now()
		meetingDate := time.Time{}
		from := make([]imap.Address, 0)
		to := make([]imap.Address, 0)
		Cc := []imap.Address{}
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

				Cc = item.Envelope.Cc
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
						} else if contentType == "text/calendar" {
							meetingDate, err = extractMeetingDates(string(b))
							isMeeting = true
							isVirtualMeeting = isVirtual(string(b))
						}
					case *mail.AttachmentHeader:
						filename, _ := h.Filename()
						log.Printf("Got attachment: %v\n", filename)
					}
				}
			}
		}

		entry := MessageEntry{
			Date:    	 	   date,
			From:    	 	   from[0].Mailbox + "@" + from[0].Host,
			FromName:    	   from[0].Name,
			To:      	 	   to[0].Mailbox + "@" + to[0].Host,
			ToName:      	   to[0].Name,
			isSent:   	  	   isSent,
			Cc:                Cc,
			MeetingDate:	   meetingDate,
			isMeeting:    	   isMeeting,
			isVirtualMeeting:  isVirtualMeeting,
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

func extractInformation(name string, email string, sentEmails int, receivedEmails int) (string, string, string) {
	// apiKey := "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"
	// ctx := context.Background()
	// client := openai.NewClient(apiKey)
	// prompt1 := "A person by the name of " + name + "has an email of " + email + ", in one word each, what company do they work at (spell it out fully) and what is their likely profession? Output format is Profession, Company"
	// prompt2 := "You sent them " + strconv.Itoa(sentEmails) + "emails and they responded with " + strconv.Itoa(sentEmails) + "email. Categorize the relationship. In your response, only respond with 'engaged,'' 'sometimes engaged' or 'unresponsive' in your response"

	// response1, err := client.CreateChatCompletion(ctx,
	// 	openai.ChatCompletionRequest{
	// 		Model: openai.GPT4,
	// 		Messages: []openai.ChatCompletionMessage{
	// 			{
	// 				Role:    openai.ChatMessageRoleUser,
	// 				Content: prompt1,
	// 			},
	// 		},
	// 		MaxTokens: 150,
	// 	},
	// )
	// if err != nil {
	// 	log.Fatal("Error generating response from OpenAI:", err)
	// }
	
	// result := response1.Choices[0].Message.Content
	// match := extractCompanyProfession(result)
	// profession, company := match[1], match[2]
	// response2, err := client.CreateChatCompletion(ctx,
	// 	openai.ChatCompletionRequest{
	// 		Model: openai.GPT4,
	// 		Messages: []openai.ChatCompletionMessage{
	// 			{
	// 				Role:    openai.ChatMessageRoleUser,
	// 				Content: prompt2,
	// 			},
	// 		},
	// 		MaxTokens: 150,
	// 	},
	// )
	// if err != nil {
	// 	log.Fatal("Error generating response from OpenAI:", err)
	// }
	
	// relationship := response2.Choices[0].Message.Content

	// return profession, company, relationship
	return "hello", "hi", "good"
}

func extractCompanyProfession(input string) []string {
	pattern := regexp.MustCompile(`(.+?),\s*(.+)`)
	match := pattern.FindStringSubmatch(input)
	return match
}

func extractMeetingDates(input string) (time.Time, error) {
	datePattern := `\bDTSTART;[^\n]*:(\d{8}T\d{6}|\d{8})\b`
	re := regexp.MustCompile(datePattern)
	matches := re.FindAllStringSubmatch(input, -1)
	var meetingDates []time.Time
	for _, match := range matches {
		if len(match) > 1 {
			dateStr := match[1]
			date, err := parseDate(dateStr)
			if err != nil {
				return time.Time{}, err
			}
			meetingDates = append(meetingDates, date)
		}
	}

	return meetingDates[0], nil
}

func parseDate(dateStr string) (time.Time, error) {
	layout := "20060102T150405"
	if len(dateStr) == 8 {
		layout = "20060102"
	}
	date, err := time.Parse(layout, dateStr)
	if err != nil {
		return time.Time{}, err
	}

	return date, nil
}

func isVirtual(rawInput string) bool {
	var locations []string

	lines := strings.Split(rawInput, "\n")
	for _, line := range lines {
		pattern := `^LOCATION(\S.*)`
		re := regexp.MustCompile(pattern)
		match := re.FindStringSubmatch(line)
		if len(match) == 2 {
			location := strings.TrimSpace(match[1])
			location = strings.Replace(location, ":", "", 1)
			location = strings.Replace(location, ";", "", 1)
			locations = append(locations, location)
		}
	}
	return locations[0] == "" || strings.HasPrefix(locations[0], "http://") || strings.HasPrefix(locations[0], "https://") || strings.HasPrefix(locations[0], "LANGUAGE")
}
func exportToCSV(entries []DatabaseEntry) error {
	file, err := os.Create("messages.csv")
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"Name", "Email", "SentEmails", "ReceivedEmails", "Profession", 
					   "Company", "Others mentioned", "Total Meetings", "Virtual Meetings", 
					   "Physical Meetings", "Frequency of Meetings", "Relationship", "LastDate"}
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
			entry.Cc,
			strconv.Itoa(entry.TotalMeetings),
			strconv.Itoa(entry.VirtualMeetings),
			strconv.Itoa(entry.PhysicalMeetings),
			strconv.FormatFloat(float64(entry.FrequencyOfMeetings), 'g', 5, 64),
			entry.Relationship,
			entry.LastDate.Format("2006-01-02 15:04:05"),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}