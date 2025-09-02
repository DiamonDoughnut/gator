package middleware

import (
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/diamondoughnut/gator/internal/config"
	sqlc "github.com/diamondoughnut/gator/internal/database"
	"github.com/google/uuid"

	_ "github.com/lib/pq"
)

type State struct {
	Db *sqlc.Queries
	CurrentCfg *config.Config
}

type Command struct {
	Name string
	Args []string
	Execute func(string) error
}

type Commands struct {
	CommandList map[string]func(*State, Command) error
}

func MiddlewareLoggedIn(handler func(s *State, cmd Command, user sqlc.User) error) func(*State, Command) error {
	return func(s *State, cmd Command) error {
		user, err := s.Db.GetUserByName(context.Background(), s.CurrentCfg.CurrentUserName)
		if err != nil {
			ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 42]: %v", sanitizeForLog(err.Error())))
		}
		return handler(s, cmd, user)
	}
}

func (c *Commands) Run(s *State, cmd Command) error {
	handler, exists := c.CommandList[cmd.Name]
	if !exists {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 51]: Command Does Not Exist"))
	}
	return handler(s, cmd)
}

func (c *Commands) Register(name string, execute func(*State, Command) error) {
	if c.CommandList == nil {
		c.CommandList = make(map[string]func(*State, Command) error)
	}
	c.CommandList[name] = execute
}

func HandlerReset(s *State, cmd Command) error {
	err := s.Db.DropUsers(context.Background())
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 66]: %v", err))
	}
	fmt.Println("Users Cleared")
	syscall.Exit(0)
	return nil
}

func HandlerUsers(s *State, cmd Command) error {
	users, err := s.Db.GetUsers(context.Background(), 10)
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 76]: %v", err))
	}
	for _, user := range users {
		if s.CurrentCfg.CurrentUserName == user.Name {
			fmt.Println("*", user.Name, "(current)")
			continue
		}
		fmt.Println("*", user.Name)
	}
	return nil
}

type RSSFeed struct {
	Channel struct {
		Title       string    `xml:"title"`
		Link        string    `xml:"link"`
		Description string    `xml:"description"`
		Item        []RSSItem `xml:"item"`
	} `xml:"channel"`
}

type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

func sanitizeForLog(input string) string {
	// Remove newlines and carriage returns to prevent log injection
	return strings.ReplaceAll(strings.ReplaceAll(input, "\n", ""), "\r", "")
}

func validateURL(feedURL string) error {
	parsedURL, err := url.Parse(feedURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("only HTTP and HTTPS URLs are allowed")
	}
	if strings.Contains(parsedURL.Host, "localhost") || strings.Contains(parsedURL.Host, "127.0.0.1") || strings.Contains(parsedURL.Host, "::1") {
		return fmt.Errorf("localhost URLs are not allowed")
	}
	return nil
}

func FetchFeed(ctx context.Context, feedURL string) (*RSSFeed, error) {
	if err := validateURL(feedURL); err != nil {
		return nil, err
	}
 // amazonq-ignore-next-line
	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 130]: %v", err))
		return nil, err
	}
	client := http.Client{
		Timeout: 30 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 138]: %v", sanitizeForLog(err.Error())))
		return nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 144]: %v", err))
		return nil, err
	}
	feed := &RSSFeed{}
	err = xml.Unmarshal(data, feed)
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 150]: %v", sanitizeForLog(err.Error())))
		return nil, err
	}
	feed.Channel.Title = html.UnescapeString(feed.Channel.Title)
	feed.Channel.Description = html.UnescapeString(feed.Channel.Description)
	return feed, nil
}

func HandlerAgg(s *State, cmd Command) error {
	if len(cmd.Args) < 1 {
		fmt.Println("Must provide a time interval")
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 161]"))
	}
	interval, err := time.ParseDuration(cmd.Args[0])
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 165]: %v", err))
	}
	if interval < time.Second * 120  {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 168]: Interval must be at least 2 minutes"))
	}
	ticker := time.NewTicker(interval)
	for ; ; <- ticker.C {
		scrapeFeeds(s)
	}
}

func HandlerAddFeed(s *State, cmd Command, user sqlc.User) error {
	if len(cmd.Args) < 2 {
		fmt.Println("Must provide name and url")
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 179]"))
	}
	name := cmd.Args[0]
	url := cmd.Args[1]

	if err := validateURL(url); err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 185]: %v", err))
	}

	newFeed, err := s.Db.CreateFeed(context.Background(), sqlc.CreateFeedParams{Name: name, Url: url, UserID: user.ID})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "violates unique constraint") || strings.Contains(err.Error(), "feeds_pkey") {
			_, err = s.Db.CreateFeedFollow(context.Background(), sqlc.CreateFeedFollowParams{ID: uuid.New(), CreatedAt: time.Now(), UpdatedAt: time.Now(), UserID: user.ID, FeedUrl: url})
			if err != nil {
				if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "violates unique constraint") {
					fmt.Printf("Already following this feed\n")
					return nil
				}
				ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 197]: %v", err))
			}
			fmt.Printf("Feed already exists - Followed\n")
			return nil
		}
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 202]: %v", err))
	}
	_, err = s.Db.CreateFeedFollow(context.Background(), sqlc.CreateFeedFollowParams{ID: uuid.New(), CreatedAt: time.Now(), UpdatedAt: time.Now(), UserID: user.ID, FeedUrl: newFeed.Url})
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 206]: %v", err))
	}
	fmt.Printf("Feed created and followed: %s\n", name)
	return nil
}

func HandlerFeeds(s *State, cmd Command) error {
	feeds, err := s.Db.GetFeeds(context.Background())
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 215]: %v", err))
	}
	for _, feed := range feeds {
		user, err := s.Db.GetUser(context.Background(), feed.UserID)
		if err != nil {
			ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 220]: %v", err))
		}
		fmt.Println(feed.Name)
		fmt.Println(feed.Url)
		fmt.Println(user.Name)
	}
	return nil
}

func HandlerFollow(s *State, cmd Command, user sqlc.User) error {
	if len(cmd.Args) < 1 {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 231]"))
	}
	if s.CurrentCfg.CurrentUserName == "" {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 234]"))
	}
	
	feed, err := s.Db.GetFeedByUrl(context.Background(), cmd.Args[0])
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 239]: %v", err))
	}
	_, err = s.Db.CreateFeedFollow(context.Background(), sqlc.CreateFeedFollowParams{ID: uuid.New(), CreatedAt: time.Now(), UpdatedAt: time.Now(), UserID: user.ID, FeedUrl: feed.Url})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "violates unique constraint") {
			fmt.Printf("Already following this feed\n")
			return nil
		}
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 247]: %v", err))
	}
	fmt.Printf("Feed: %v\nUser: %v\n", feed.Name, user.Name)
	return nil
}

func HandlerFollowing(s *State, cmd Command, user sqlc.User) error {
	
	follows, err := s.Db.GetFeedFollowsForUser(context.Background(), user.ID)
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 257]: %v", err))
	}
	for _, follow := range follows {
		fmt.Println(follow.FeedName)
	}
	return nil
}

func HandlerLogin(s *State, cmd Command) error {
	if len(cmd.Args) < 1 {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 267]"))
	}
	usr := cmd.Args[0]
	_, err := s.Db.GetUserByName(context.Background(), usr)
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 272]: %v", err))
	}
	s.CurrentCfg.CurrentUserName = usr
	err = config.SetUser(usr)
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 277]: %v", err))
	}
	fmt.Println("Logged in as", usr)
	return nil
}

func HandlerRegister(s *State, cmd Command) error {
	if len(cmd.Args) < 1 {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 285]"))
	}
	usr := cmd.Args[0]
	_, err := s.Db.GetUserByName(context.Background(), usr)
	if err == nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 290]: %v", err))
	}
	_, err = s.Db.CreateUser(context.Background(), sqlc.CreateUserParams{ID: uuid.New(), CreatedAt: time.Now(), UpdatedAt: time.Now(), Name: usr})
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 294]: %v", err))
	}
	fmt.Println("Registered user:", usr)
	s.CurrentCfg.CurrentUserName = usr
	err = config.SetUser(usr)
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 300]: %v", err))
	}
	fmt.Println("Logged in as", usr)
	return nil
}

func HandlerUnfollow(s *State, cmd Command, user sqlc.User) error {
	if len(cmd.Args) < 1 {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 308]"))
	}
	err := s.Db.DeleteFeedFollowByUserAndFeedUrl(context.Background(), sqlc.DeleteFeedFollowByUserAndFeedUrlParams{UserID: user.ID, FeedUrl: cmd.Args[0]})
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 312]: %v", err))
	}
	return nil
}

func scrapeFeeds(s *State) {
	feed, err := s.Db.GetNextFeedToFetch(context.Background())
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 320]: %v", err))
	}
	err = s.Db.MarkFeedFetched(context.Background(), feed.Url)
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 324]: %v", err))
	}
	feedData, err := FetchFeed(context.Background(), feed.Url)
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 328]: %v", err))
	}
	feedItems := feedData.Channel.Item
	for _, item := range feedItems {
		title := item.Title
		url := item.Link
		description := item.Description
		var published_at_xml XMLtime
		err := xml.Unmarshal([]byte(item.PubDate), &published_at_xml)
		if err != nil {
			if err != io.EOF {
				ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 340]: %v", err))
			}
		}
		published_at := sql.NullTime{Time: published_at_xml.Time, Valid: !published_at_xml.Time.IsZero()}
		feed_url := feed.Url
		
		_, err = s.Db.CreatePost(context.Background(), sqlc.CreatePostParams{CreatedAt: time.Now(), UpdatedAt: time.Now(), Title: title, Url: url, Description: description, PublishedAt: published_at, FeedUrl: feed_url})
		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "violates unique constraint") {
				continue
			}
			ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 352]: %v", err))
		}
	}
	fmt.Println("Cycling feed scraper")
}

func HandlerBrowse(s *State, cmd Command, user sqlc.User) error {
	var limit int
	if len(cmd.Args) < 1 {
		limit = 2
	} else {
		limit, _ = strconv.Atoi(cmd.Args[0])
	}
	posts, err := s.Db.GetPostsForUser(context.Background(), sqlc.GetPostsForUserParams{UserID: user.ID, Limit: int32(limit)})
	if err != nil {
		ThrowError(fmt.Errorf("[GATOR: CMDS.GO: LINE 365]: %v", err))
	}
	for _, post := range posts {
		fmt.Println(post.Title)
		fmt.Println(post.Url)
		fmt.Println(post.Description)
		fmt.Println(post.PublishedAt.Time)
		fmt.Println(post.FeedUrl)
		fmt.Println()
	}
	return nil
}

type XMLtime struct {
	time.Time
}

func (t *XMLtime) UnmarshalText(text []byte) error {
	formats := []string{
		"2006-01-02T15:04:05Z07:00", 
		"2006-01-02",                
		"01/02/2006",                
		"02-Jan-06",
		"Tue, 02 Sep 2025 04:30:00 +0000",
	}

	dateStr := strings.TrimSpace(string(text))

	for _, f := range formats {
		parsedTime, err := time.Parse(f, dateStr)
		if err == nil {
			*t = XMLtime{parsedTime}
			return nil
		}
	}

	return fmt.Errorf("unable to parse date: %s", dateStr)
}

func ThrowError(err error) {
	log.Fatal(err)
}
