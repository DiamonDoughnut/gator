package main

import (
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/diamondoughnut/gator/internal/config"
	sqlc "github.com/diamondoughnut/gator/internal/database"
	"github.com/google/uuid"

	_ "github.com/lib/pq"
)

type state struct {
	db *sqlc.Queries
	currentCfg *config.Config
}

type command struct {
	name string
	args []string
	execute func(string) error
}

type Commands struct {
	command_list map[string]func(*state, command) error
}

func main() {
	
	currentConfig, err := config.Read()
	if err != nil {
		fmt.Printf("Error reading config: %v\n", err)
		syscall.Exit(1)
	}
	currentState := state{}
	currentState.currentCfg = &currentConfig
	db, err := sql.Open("postgres", currentConfig.DbUrl)
	if err != nil {
		fmt.Printf("Error accessing database: %v\n", err)
		syscall.Exit(1)
	}
	dbQueries := sqlc.New(db)
	currentState.db = dbQueries
	commands := Commands{}
	commands.register("login", handlerLogin)
	commands.register("register", handlerRegister)
	commands.register("reset", handlerReset)
	commands.register("users", handlerUsers)
	commands.register("agg", handlerAgg)
	args := os.Args
	if len(args) < 2 {
		fmt.Println("No command provided")
		syscall.Exit(1)
	}
	commandArg := args[1]
	commandArgs := args[2:]
	err = commands.run(&currentState, command{commandArg, commandArgs, nil})
	if err != nil {
		fmt.Printf("Error running command: %v\n", err)
		syscall.Exit(1)
	}
	if len(commandArgs) > 0 {
		config.SetUser(commandArgs[0])
	}
}

func handlerLogin(s *state, cmd command) error {
	if len(cmd.args) < 1 {
		fmt.Println("no username provided")
		syscall.Exit(1)
	}
	usr := cmd.args[0]
	_, err := s.db.GetUserByName(context.Background(), usr)
	if err != nil {
		fmt.Printf("User %s not found\n", usr)
		syscall.Exit(1)
	}
	s.currentCfg.CurrentUserName = usr
	fmt.Println("Logged in as", usr)
	return nil
}

func handlerRegister(s *state, cmd command) error {
	if len(cmd.args) < 1 {
		fmt.Println("no username provided")
		syscall.Exit(1)
	}
	usr := cmd.args[0]
	_, err := s.db.GetUserByName(context.Background(), usr)
	if err == nil {
		fmt.Println("User already exists")
		syscall.Exit(1)
	}
	_, err = s.db.CreateUser(context.Background(), sqlc.CreateUserParams{ID: uuid.New(), CreatedAt: time.Now(), UpdatedAt: time.Now(), Name: usr})
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	fmt.Println("Registered user:", usr)
	s.currentCfg.CurrentUserName = usr
	fmt.Println("Logged in as", usr)
	return nil
}

func (c *Commands) run(s *state, cmd command) error {
	handler, exists := c.command_list[cmd.name]
	if !exists {
		return errors.New("command not found")
	}
	return handler(s, cmd)
}

func (c *Commands) register(name string, execute func(*state, command) error) {
	if c.command_list == nil {
		c.command_list = make(map[string]func(*state, command) error)
	}
	c.command_list[name] = execute
}

func handlerReset(s *state, cmd command) error {
	err := s.db.DropUsers(context.Background())
	if err != nil {
		fmt.Printf("Error dropping users: %v\n", err)
		syscall.Exit(1)
	}
	fmt.Println("Users Cleared")
	syscall.Exit(0)
	return nil
}

func handlerUsers(s *state, cmd command) error {
	users, err := s.db.GetUsers(context.Background(), 10)
	if err != nil {
		fmt.Printf("Error listing users: %v\n", err)
		syscall.Exit(1)
	}
	for _, user := range users {
		if s.currentCfg.CurrentUserName == user.Name {
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

func handlerAgg(s *state, cmd command) error {
	feed, err := fetchFeed(context.Background(), "https://www.wagslane.dev/index.xml")
	if err != nil {
		return err
	}
	fmt.Printf("%v\n", feed)
	return nil
}