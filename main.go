package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Bot parameters
var (
	GuildID  = flag.String("guild", "", "Test guild ID. If not passed - bot registers commands globally")
	BotToken = os.Getenv("TOKEN")
)

type RedditPost []struct {
	Kind string `json:"kind"`
	Data struct {
		Children []struct {
			Kind string `json:"kind"`
			Data struct {
				Subreddit string `json:"subreddit"`
				Title     string `json:"title,omitempty"`
				URL       string `json:"url,omitempty"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

type Subreddit struct {
	Kind string `json:"kind"`
	Data struct {
		URL string `json:"url"`
	} `json:"data"`
}

// map of ChannelID to a slice of Subreddits
var ChannelSubreddits = map[string][]string{}

func getJson(url string, target interface{}) error {
	// Create a new HTTP client with a timeout of 10 seconds
	var myClient = http.Client{Timeout: 10 * time.Second}

	// Build a GET request to the meal API endpoint
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Add the required headers to the request
	req.Header.Set("User-Agent", "RaunchBot")

	// Send the request and store the response
	r, getErr := myClient.Do(req)
	if getErr != nil {
		log.Fatal(getErr)
	}

	// Close the response body when done
	defer r.Body.Close()

	// Read the response body
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Fatal(err)
	}

	// Unmarshal the JSON response into the target interface
	err = json.Unmarshal(b, &target)
	if err != nil {
		log.Printf("Error unmarshalling JSON: %v", err)
	}

	return json.Unmarshal(b, target)
}

var s *discordgo.Session

func init() { flag.Parse() }

func init() {
	var err error
	if BotToken == "" {
		log.Fatal("Token cannot be empty")
	}

	s, err = discordgo.New("Bot " + BotToken)
	if err != nil {
		log.Fatalf("Invalid bot parameters: %v", err)
	}
}

var (
	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "random",
			Description: "random post from a random subreddit from this channel's list",
		},
		{
			Name:        "add",
			Description: "add a subreddit to this channel's list",
			Options: []*discordgo.ApplicationCommandOption{

				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "subreddit",
					Description: "enter subreddit name",
					Required:    true,
				},
			},
		},
		{
			Name:        "remove",
			Description: "remove a subreddit from this channel's list",
			Options: []*discordgo.ApplicationCommandOption{

				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "subreddit",
					Description: "enter subreddit name",
					Required:    true,
				},
			},
		},
		{
			Name:        "list",
			Description: "lists of available subreddits in this channel's list",
			Options:     []*discordgo.ApplicationCommandOption{},
		},
		{
			Name:        "sub",
			Description: "random post from a specific subreddit",
			Options: []*discordgo.ApplicationCommandOption{

				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "subreddit",
					Description: "enter subreddit name",
					Required:    true,
				},
			},
		},
	}
	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"random": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			var msg string
			// If the []subreddits is empty, it will return an error
			if len(ChannelSubreddits[i.ChannelID]) == 0 {
				msg = "There are no subreddits in this channel's list."
			} else {

				// Seed the random number generator & get a random index from the slice
				rand.Seed(time.Now().UnixNano())
				randIndex := rand.Intn(len(ChannelSubreddits[i.ChannelID]))

				// Query the API for a random post from a random subreddit
				randomRedditPost := RedditPost{}
				getJson("https://reddit.com/r/"+ChannelSubreddits[i.ChannelID][randIndex]+"/random.json", &randomRedditPost)
				// if randomRedditPost is empty, return an error
				if len(randomRedditPost) == 0 {
					msg = "`r/" + ChannelSubreddits[i.ChannelID][randIndex] + "`" + " is not a supported subreddit."
				} else {
					msg = randomRedditPost[0].Data.Children[0].Data.Title + "\n`r/" + randomRedditPost[0].Data.Children[0].Data.Subreddit + "`\n" + randomRedditPost[0].Data.Children[0].Data.URL
				}
			}
			// Respond with the post's title and URL
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: msg,
				},
			})
		},
		"add": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			// Query user for subreddit name
			subreddit := i.ApplicationCommandData().Options[0].StringValue()

			// Setup the response data
			var msg string

			// if the subreddit is already contained in the slice, do nothing
			if contains(ChannelSubreddits[i.ChannelID], subreddit) {
				msg = "The subreddit " + subreddit + " is already on this channel's list."
			} else {
				subredditCheck := Subreddit{}
				getJson("https://reddit.com/r/"+subreddit+"/about.json", &subredditCheck)
				// If the subreddit exists, add it to the slice
				if subredditCheck.Data.URL == "" {
					msg = "The subreddit " + subreddit + " was not found. Try again."
				} else {
					ChannelSubreddits[i.ChannelID] = append(ChannelSubreddits[i.ChannelID], subreddit)
					msg = subreddit + " has been added to the channel's list."
				}
			}
			// Respond with the message
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: msg,
				},
			})

		},
		"list": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			// Setup the response data
			var msg string

			// If the []subreddits is empty, it will return an error
			if len(ChannelSubreddits[i.ChannelID]) == 0 {
				msg = "There are no subreddits on this channel's list."
			} else {
				msg = "The following subreddits are available:\n" + "```\n" + strings.Join(ChannelSubreddits[i.ChannelID], "\n") + "\n```"
			}

			// Respond with the message
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: msg,
				},
			})
		},
		"remove": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			// Query user for subreddit name
			subreddit := i.ApplicationCommandData().Options[0].StringValue()

			// Setup the response data
			var msg string

			// If subreddit is not in []subreddits, return error
			if !contains(ChannelSubreddits[i.ChannelID], subreddit) {
				msg = subreddit + " is not on this channel's list."
			} else {
				// Remove subreddit from []subreddits
				ChannelSubreddits[i.ChannelID] = remove(ChannelSubreddits[i.ChannelID], subreddit)
				msg = subreddit + " has been removed from this channel's list."
			}

			// Respond with the message
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: msg,
				},
			})
		},
		"sub": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			// Query user for subreddit name
			subreddit := i.ApplicationCommandData().Options[0].StringValue()

			// Setup the response data
			var msg string

			subredditCheck := Subreddit{}
			getJson("https://reddit.com/r/"+subreddit+"/about.json", &subredditCheck)

			// If the subreddit exists, get a random post from it
			if subredditCheck.Data.URL == "" {
				msg = "The subreddit " + subreddit + " was not found. Try again."
			} else {
				// Query the API for a random post from a random subreddit
				randomRedditPost := RedditPost{}
				getJson("https://reddit.com/r/"+subreddit+"/random.json", &randomRedditPost)

				// if randomRedditPost is empty, return an error
				if len(randomRedditPost) == 0 {
					msg = "`r/" + subreddit + "`" + " is not a supported subreddit."
				} else {
					msg = randomRedditPost[0].Data.Children[0].Data.Title + "\n`r/" + randomRedditPost[0].Data.Children[0].Data.Subreddit + "`\n" + randomRedditPost[0].Data.Children[0].Data.URL
				}
			}

			// Respond with the post's title and URL
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: msg,
				},
			})
		},
	}
)

func init() {
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})
}

func main() {
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Println("Bot is up!")
	})
	err := s.Open()
	if err != nil {
		log.Fatalf("Cannot open the session: %v", err)
	}

	for _, v := range commands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, *GuildID, v)
		if err != nil {
			log.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
	}

	defer s.Close()
	stop := make(chan os.Signal)
	signal.Notify(stop, os.Interrupt)
	<-stop

	log.Println("Gracefully shutdowning; Cleaning up commands")

	for _, v := range commands {
		s.ApplicationCommandDelete(s.State.User.ID, *GuildID, v.Name)
	}
}

// helper functions
func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

func remove(s []string, r string) []string {
	for i, v := range s {
		if v == r {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}
