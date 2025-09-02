package middleware

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"syscall"
	"time"
	"errors"

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
	Command_list map[string]func(*State, Command) error
}

func MiddlewareLoggedIn(handler func(s *State, cmd Command, user sqlc.User) error) func(*State, Command) error {
	return func(s *State, cmd Command) error {
		user, err := s.Db.GetUserByName(context.Background(), s.CurrentCfg.CurrentUserName)
		if err != nil {
			return err
		}
		return handler(s, cmd, user)
	}
}

func (c *Commands) Run(s *State, cmd Command) error {
	handler, exists := c.Command_list[cmd.Name]
	if !exists {
		return errors.New("command not found")
	}
	return handler(s, cmd)
}

func (c *Commands) Register(name string, execute func(*State, Command) error) {
	if c.Command_list == nil {
		c.Command_list = make(map[string]func(*State, Command) error)
	}
	c.Command_list[name] = execute
}

func HandlerReset(s *State, cmd Command) error {
	err := s.Db.DropUsers(context.Background())
	if err != nil {
		fmt.Printf("Error dropping users: %v\n", err)
		syscall.Exit(1)
	}
	fmt.Println("Users Cleared")
	syscall.Exit(0)
	return nil
}

func HandlerUsers(s *State, cmd Command) error {
	users, err := s.Db.GetUsers(context.Background(), 10)
	if err != nil {
		fmt.Printf("Error listing users: %v\n", err)
		syscall.Exit(1)
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

func fetchFeed(ctx context.Context, feedURL string) (*RSSFeed, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		return nil, err
	}
	client := http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	feed := &RSSFeed{}
	err = xml.Unmarshal(data, feed)
	if err != nil {
		return nil, err
	}
	feed.Channel.Title = html.UnescapeString(feed.Channel.Title)
	feed.Channel.Description = html.UnescapeString(feed.Channel.Description)
	return feed, nil
}

func HandlerAgg(s *State, cmd Command) error {
	feed, err := fetchFeed(context.Background(), "https://www.wagslane.dev/index.xml")
	if err != nil {
		return err
	}
	fmt.Printf("%v\n", feed)
	return nil
}

func HandlerAddFeed(s *State, cmd Command, user sqlc.User) error {
	if len(cmd.Args) < 2 {
		fmt.Println("Must provide name and url")
		syscall.Exit(1)
	}
	name := cmd.Args[0]
	url := cmd.Args[1]

	newFeed, err := s.Db.CreateFeed(context.Background(), sqlc.CreateFeedParams{Name: name, Url: url, UserID: user.ID})
	if err != nil {
		// Check if it's a duplicate URL error (feeds table uses url as primary key)
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "violates unique constraint") || strings.Contains(err.Error(), "feeds_pkey") {
			_, err = s.Db.CreateFeedFollow(context.Background(), sqlc.CreateFeedFollowParams{ID: uuid.New(), CreatedAt: time.Now(), UpdatedAt: time.Now(), UserID: user.ID, FeedUrl: url})
			if err != nil {
				if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "violates unique constraint") {
					fmt.Printf("Already following this feed\n")
					return nil
				}
				fmt.Printf("Feed already exists - Error creating follow: %v\n", err)
				return err
			}
			fmt.Printf("Feed already exists - Followed\n")
			return nil
		}
		fmt.Printf("Error creating feed: %v\n", err)
		return err
	}
	_, err = s.Db.CreateFeedFollow(context.Background(), sqlc.CreateFeedFollowParams{ID: uuid.New(), CreatedAt: time.Now(), UpdatedAt: time.Now(), UserID: user.ID, FeedUrl: newFeed.Url})
	if err != nil {
		return err
	}
	fmt.Printf("Feed created and followed: %s\n", name)
	return nil
}

func HandlerFeeds(s *State, cmd Command) error {
	feeds, err := s.Db.GetFeeds(context.Background())
	if err != nil {
		return err
	}
	for _, feed := range feeds {
		user, err := s.Db.GetUser(context.Background(), feed.UserID)
		if err != nil {
			return err
		}
		fmt.Println(feed.Name)
		fmt.Println(feed.Url)
		fmt.Println(user.Name)
	}
	return nil
}

func HandlerFollow(s *State, cmd Command, user sqlc.User) error {
	if len(cmd.Args) < 1 {
		fmt.Println("Must provide feed url")
		syscall.Exit(1)
	}
	if s.CurrentCfg.CurrentUserName == "" {
		fmt.Println("No user logged in. Please login first.")
		syscall.Exit(1)
	}
	
	usr_id := user.ID
	feed, err := s.Db.GetFeedByUrl(context.Background(), cmd.Args[0])
	if err != nil {
		fmt.Printf("Error getting feed: %v\n", err)
		return err
	}
	feed_url := feed.Url
	_, err = s.Db.CreateFeedFollow(context.Background(), sqlc.CreateFeedFollowParams{ID: uuid.New(), CreatedAt: time.Now(), UpdatedAt: time.Now(), UserID: usr_id, FeedUrl: feed_url})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "violates unique constraint") {
			fmt.Printf("Already following this feed\n")
			return nil
		}
		fmt.Printf("Error creating feed follow: %v\n", err)
		return err
	}
	fmt.Printf("Feed: %v\nUser: %v\n", feed.Name, user.Name)
	return nil
}

func HandlerFollowing(s *State, cmd Command, user sqlc.User) error {
	
	follows, err := s.Db.GetFeedFollowsForUser(context.Background(), user.ID)
	if err != nil {
		fmt.Printf("Error getting follows: %v\n", err)
		return err
	}
	for _, follow := range follows {
		fmt.Println(follow.FeedName)
	}
	return nil
}

func HandlerLogin(s *State, cmd Command) error {
	if len(cmd.Args) < 1 {
		fmt.Println("no username provided")
		syscall.Exit(1)
	}
	usr := cmd.Args[0]
	_, err := s.Db.GetUserByName(context.Background(), usr)
	if err != nil {
		fmt.Printf("User %s not found\n", usr)
		syscall.Exit(1)
	}
	s.CurrentCfg.CurrentUserName = usr
	err = config.SetUser(usr)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Println("Logged in as", usr)
	return nil
}

func HandlerRegister(s *State, cmd Command) error {
	if len(cmd.Args) < 1 {
		fmt.Println("no username provided")
		syscall.Exit(1)
	}
	usr := cmd.Args[0]
	_, err := s.Db.GetUserByName(context.Background(), usr)
	if err == nil {
		fmt.Println("User already exists")
		syscall.Exit(1)
	}
	_, err = s.Db.CreateUser(context.Background(), sqlc.CreateUserParams{ID: uuid.New(), CreatedAt: time.Now(), UpdatedAt: time.Now(), Name: usr})
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	fmt.Println("Registered user:", usr)
	s.CurrentCfg.CurrentUserName = usr
	err = config.SetUser(usr)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Println("Logged in as", usr)
	return nil
}

func HandlerUnfollow(s *State, cmd Command, user sqlc.User) error {
	if len(cmd.Args) < 1 {
		fmt.Println("Must provide feed url")
		syscall.Exit(1)
	}
	err := s.Db.DeleteFeedFollowByUserAndFeedUrl(context.Background(), sqlc.DeleteFeedFollowByUserAndFeedUrlParams{UserID: user.ID, FeedUrl: cmd.Args[0]})
	if err != nil {
		fmt.Printf("Error deleting feed follow: %v\n", err)
		return err
	}
	return nil
}