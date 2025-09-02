# Gator RSS Aggregator

A command-line RSS feed aggregator built in Go that allows users to manage and monitor RSS feeds with automatic periodic fetching.

## Features

- **User Management**: Register, login, and manage multiple users
- **Feed Management**: Add, follow, unfollow, and list RSS feeds
- **Automatic Aggregation**: Periodically fetch and display new RSS items
- **Database Storage**: PostgreSQL backend with SQLC for type-safe queries
- **Security**: Built-in protections against SSRF attacks and log injection
- **Multi-user Support**: Each user can follow their own set of feeds

## Prerequisites

- Go 1.19 or higher
- PostgreSQL database
- [SQLC](https://sqlc.dev/) for code generation
- [Goose](https://github.com/pressly/goose) for database migrations

## Installation

1. Clone the repository:
```bash
git clone https://github.com/yourusername/gator.git
cd gator
```

2. Install dependencies:
```bash
go mod tidy
```

3. Set up your PostgreSQL database and run migrations:
```bash
goose -dir sql/schema postgres "your_connection_string" up
```

4. Create your config file at `~/gatorconfig.json`:
```json
{
  "db_url": "postgres://username:password@localhost/dbname?sslmode=disable",
  "current_user_name": ""
}
```

5. Build the application:
```bash
go build -o gator
```

## Usage

### User Management

Register a new user:
```bash
./gator register <username>
```

Login as an existing user:
```bash
./gator login <username>
```

List all users:
```bash
./gator users
```

Reset all users (clears database):
```bash
./gator reset
```

### Feed Management

Add a new RSS feed:
```bash
./gator addfeed "Feed Name" "https://example.com/rss.xml"
```

List all feeds:
```bash
./gator feeds
```

Follow an existing feed:
```bash
./gator follow "https://example.com/rss.xml"
```

Unfollow a feed:
```bash
./gator unfollow "https://example.com/rss.xml"
```

List feeds you're following:
```bash
./gator following
```

### RSS Aggregation

Start automatic RSS aggregation with specified interval:
```bash
./gator agg <time_interval>
```

Examples:
```bash
./gator agg 300s    # Every 5 minutes
./gator agg 1h      # Every hour
./gator agg 30m     # Every 30 minutes
```

**Recommended intervals:**
- Development/Testing: `300s` (5 minutes)
- Production: `1h` (1 hour) or `30m` (30 minutes)

## Project Structure

```
gator/
├── main.go                     # Application entry point
├── internal/
│   ├── config/                 # Configuration management
│   │   └── config.go
│   ├── database/               # Generated SQLC code
│   │   ├── db.go
│   │   ├── models.go
│   │   └── *.sql.go
│   └── middleware/             # Command handlers and business logic
│       └── cmds.go
├── sql/
│   ├── queries/                # SQL queries for SQLC
│   │   ├── users.sql
│   │   ├── feeds.sql
│   │   └── feed_follows.sql
│   └── schema/                 # Database migrations
│       ├── 001_users.sql
│       ├── 002_feeds.sql
│       └── 003_feed_follows.sql
├── sqlc.yaml                   # SQLC configuration
├── go.mod
├── go.sum
└── README.md
```

## Database Schema

The application uses three main tables:

- **users**: Store user information with UUID primary keys
- **feeds**: Store RSS feed URLs and metadata
- **feed_follows**: Junction table linking users to their followed feeds

## Security Features

- **SSRF Protection**: URL validation prevents requests to localhost/internal networks
- **Input Sanitization**: Log injection protection for all user inputs
- **Secure File Permissions**: Config files use restrictive 0600 permissions
- **HTTP Timeouts**: 30-second timeout prevents hanging requests

## Development

### Code Generation

Regenerate database code after schema changes:
```bash
sqlc generate
```

### Database Migrations

Create a new migration:
```bash
goose -dir sql/schema create migration_name sql
```

Apply migrations:
```bash
goose -dir sql/schema postgres "connection_string" up
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests and ensure code quality
5. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- Built with [SQLC](https://sqlc.dev/) for type-safe SQL
- Database migrations powered by [Goose](https://github.com/pressly/goose)
- RSS parsing using Go's built-in XML package