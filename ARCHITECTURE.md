# Architecture Diagram

## System Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        Client Layer                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                  │
│  │   Web    │  │  Mobile  │  │   API    │                  │
│  │ Browser  │  │   App    │  │  Client  │                  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘                  │
│       │             │              │                         │
│       └─────────────┴──────────────┘                         │
│                     │                                        │
│                  HTTP/WS                                     │
└─────────────────────┼────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                    Go Application Layer                      │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │                   main.go                              │ │
│  │           (Application Entry Point)                    │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │              Middleware Layer                          │ │
│  │  ┌──────────────────┐  ┌──────────────────┐           │ │
│  │  │   CORS (cors.go) │  │  Auth (auth.go)  │           │ │
│  │  └──────────────────┘  └──────────────────┘           │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │                 Router (gorilla/mux)                   │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │                Handler Layer                           │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐            │ │
│  │  │ auth.go  │  │ data.go  │  │machine.go│            │ │
│  │  └──────────┘  └──────────┘  └──────────┘            │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │              Service Layer                             │ │
│  │  ┌──────────────────┐  ┌──────────────────┐           │ │
│  │  │   WebSocket Hub  │  │  Database Pool   │           │ │
│  │  │   (hub.go)       │  │  (database.go)   │           │ │
│  │  └──────────────────┘  └──────────────────┘           │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │               Model Layer                              │ │
│  │              (models/user.go)                          │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                      │
                      │
         ┌────────────┴────────────┐
         │                         │
         ▼                         ▼
┌──────────────────┐     ┌──────────────────┐
│   MySQL/MariaDB  │     │  WebSocket Clients│
│   Database       │     │  (Real-time)      │
│                  │     │                   │
│ - kabu_users     │     │ - Machine Status  │
│ - Machine Tables │     │ - Data Updates    │
│ - Reports        │     │ - Fault Alerts    │
└──────────────────┘     └──────────────────┘
```

## Request Flow

### Authentication Flow

```
Client Request
     │
     ▼
┌─────────────────────────┐
│  POST /api/login        │
│  {username, password}   │
└──────────┬──────────────┘
           │
           ▼
┌─────────────────────────┐
│  handlers.HandleLogin   │
│  - Validate request     │
│  - Query database       │
│  - Verify credentials   │
└──────────┬──────────────┘
           │
           ▼
┌─────────────────────────┐
│  middleware.GenerateToken│
│  - Create JWT claims    │
│  - Sign token           │
└──────────┬──────────────┘
           │
           ▼
┌─────────────────────────┐
│  Set HTTP-only Cookie   │
│  - Name: auth_token     │
│  - MaxAge: 15 minutes   │
│  - HttpOnly: true       │
│  - SameSite: Lax        │
└──────────┬──────────────┘
           │
           ▼
┌─────────────────────────┐
│  Return Response        │
│  {user, token, message} │
└─────────────────────────┘
```

### Protected Route Flow

```
Client Request with Token
     │
     ▼
┌─────────────────────────┐
│  Middleware:            │
│  AuthenticateToken      │
└──────────┬──────────────┘
           │
           ▼
     ┌─────┴─────┐
     │           │
     ▼           ▼
┌─────────┐ ┌──────────┐
│ Cookie  │ │  Header  │
│ Check   │ │  Check   │
└────┬────┘ └────┬─────┘
     │           │
     └─────┬─────┘
           │
           ▼
┌─────────────────────────┐
│  Validate JWT Token     │
│  - Check signature      │
│  - Check expiration     │
│  - Extract claims       │
└──────────┬──────────────┘
           │
           ▼
     ┌─────┴─────┐
     │           │
     ▼           ▼
┌─────────┐ ┌──────────┐
│ Valid   │ │ Invalid  │
│ ───────►│ │ ───────► │
│ Add to  │ │ 403 Error│
│ Context │ │          │
└────┬────┘ └──────────┘
     │
     ▼
┌─────────────────────────┐
│  Call Protected Handler │
│  req.Context() has user │
└─────────────────────────┘
```

### WebSocket Connection Flow

```
Client WebSocket Request
     │
     ▼
┌─────────────────────────┐
│  websocket.Upgrader     │
│  Upgrade HTTP to WS     │
└──────────┬──────────────┘
           │
           ▼
┌─────────────────────────┐
│  Create Client Object   │
│  - ID                   │
│  - Conn                 │
│  - Send channel         │
└──────────┬──────────────┘
           │
           ▼
┌─────────────────────────┐
│  Register with Hub      │
│  hub.Register <- client │
└──────────┬──────────────┘
           │
           ▼
┌─────────────────────────┐
│  Start Goroutines       │
│  - readPump()           │
│  - writePump()          │
└─────────────────────────┘

Concurrent Operations:
┌─────────────────────────────────────┐
│           WebSocket Hub             │
│                                     │
│  ┌──────────────────────────────┐  │
│  │  Run() - Main Loop           │  │
│  │  select {                    │  │
│  │    case client := <-Register │  │
│  │      add client              │  │
│  │    case client := <-Unregister│ │
│  │      remove client           │  │
│  │    case msg := <-Broadcast   │  │
│  │      send to all clients     │  │
│  │  }                           │  │
│  └──────────────────────────────┘  │
└─────────────────────────────────────┘
```

### Database Query Flow

```
Handler Request
     │
     ▼
┌─────────────────────────┐
│  database.SafeQuery()   │
│  - Check connection     │
│  - Log query start      │
└──────────┬──────────────┘
           │
           ▼
┌─────────────────────────┐
│  Execute Query          │
│  db.Query(query, args)  │
└──────────┬──────────────┘
           │
           ▼
     ┌─────┴─────┐
     │           │
     ▼           ▼
┌─────────┐ ┌──────────┐
│ Success │ │  Error   │
│         │ │          │
│ Log time│ │ Log error│
│ Return  │ │ Return   │
└─────────┘ └──────────┘
```

## Component Interaction

### Main Application Lifecycle

```
main.main()
    │
    ├─► Load environment variables
    │
    ├─► Initialize database pool
    │     └─► Configure connections
    │     └─► Test connectivity
    │
    ├─► Create WebSocket hub
    │     └─► Start hub goroutine
    │
    ├─► Setup router & middleware
    │     ├─► EnableCORS
    │     ├─► Define routes
    │     └─► Apply authentication
    │
    ├─► Start HTTP server (goroutine)
    │
    └─► Wait for signals
          ├─► SIGINT/SIGTERM received
          ├─► Graceful shutdown
          ├─► Close database
          └─► Exit
```

### Configuration Management

```
config.GetConfig() (singleton)
    │
    ├─► Load from .env or environment
    │
    ├─► DB_HOST
    ├─► DB_PORT
    ├─► DB_USER
    ├─► DB_PASSWORD
    ├─► DB_NAME
    ├─► JWT_SECRET
    ├─► PORT
    └─► CORS_ORIGIN
```

## Security Layers

```
┌─────────────────────────────────────┐
│         Request from Client         │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  Layer 1: CORS Middleware           │
│  - Validate origin                  │
│  - Set CORS headers                 │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  Layer 2: Authentication (if needed)│
│  - Validate JWT token               │
│  - Check cookie or header           │
│  - Extract user claims              │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  Layer 3: Handler Logic             │
│  - Business logic                   │
│  - Input validation                 │
│  - Parameterized queries            │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  Layer 4: Database Access           │
│  - Connection pooling               │
│  - Safe query execution             │
│  - Resource cleanup                 │
└─────────────────────────────────────┘
```

## Data Models

### User Model
```go
type User struct {
    ID            int
    AccountType   string
    FirstName     string
    LastName      string
    Username      string
    Email         string
    PhoneNumber   string
    Company       string
    Password      string  // Never returned in JSON
    MonitorAccess string
    Location      string
    CreatedAt     time.Time
}
```

### JWT Claims
```go
type UserClaims struct {
    Username    string
    AccountType string
    UserID      int
    jwt.RegisteredClaims  // Includes Expiration, IssuedAt
}
```

### Machine Status
```go
type MachineStatus struct {
    MachineStatus  bool
    CoolingStatus  bool
    InternetStatus bool
    MachineType    string
    Priority       string
    ResponseType   string
    NoNewData      bool
    RecordID       int
    LastUpdate     string
    // ... more fields
}
```

## Deployment Architecture

### Development
```
Local Machine
    └─► go run main.go
        └─► Port 8080
        └─► Connects to remote MySQL
```

### Production
```
Server/Container
    └─► grain_backend.exe
        ├─► Reverse Proxy (nginx)
        │   └─► SSL/TLS termination
        │
        ├─► Load Balancer (optional)
        │   └─► Multiple instances
        │
        └─► MySQL Database
            └─► Connection pool
            └─► Read replicas (optional)
```

---

This architecture provides:
- **Scalability** - Horizontal scaling possible
- **Reliability** - Proper error handling and recovery
- **Security** - Multiple security layers
- **Performance** - Optimized connection pooling and concurrency
- **Maintainability** - Clear separation of concerns
