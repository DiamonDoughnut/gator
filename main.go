package main

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/diamondoughnut/gator/internal/config"
	sqlc "github.com/diamondoughnut/gator/internal/database"
	"github.com/diamondoughnut/gator/internal/middleware"

	_ "github.com/lib/pq"
)



func main() {
	
	currentConfig, err := config.Read()
	if err != nil {
		fmt.Printf("Error reading config: %v\n", err)
		os.Exit(1)
	}
	currentState := middleware.State{}
	currentState.CurrentCfg = &currentConfig
	db, err := sql.Open("postgres", currentConfig.DbUrl)
	if err != nil {
		fmt.Printf("Error accessing database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	dbQueries := sqlc.New(db)
	currentState.Db = dbQueries
	commands := middleware.Commands{}
	commands.Register("login", middleware.HandlerLogin)
	commands.Register("register", middleware.HandlerRegister)
	commands.Register("reset", middleware.HandlerReset)
	commands.Register("users", middleware.HandlerUsers)
	commands.Register("agg", middleware.HandlerAgg)
	commands.Register("addfeed", middleware.MiddlewareLoggedIn(middleware.HandlerAddFeed))
	commands.Register("feeds", middleware.HandlerFeeds)
	commands.Register("follow", middleware.MiddlewareLoggedIn(middleware.HandlerFollow))
	commands.Register("following", middleware.MiddlewareLoggedIn(middleware.HandlerFollowing))
	commands.Register("unfollow", middleware.MiddlewareLoggedIn(middleware.HandlerUnfollow))
	commands.Register("browse", middleware.MiddlewareLoggedIn(middleware.HandlerBrowse))
	args := os.Args
	if len(args) < 2 {
		fmt.Println("No command provided")
		os.Exit(1)
	}
	commandArg := args[1]
	commandArgs := args[2:]
	err = commands.Run(&currentState, middleware.Command{Name: commandArg, Args: commandArgs, Execute: nil})
	if err != nil {
		fmt.Printf("Error running command: %v\n", err)
		os.Exit(1)
	}
}

