You are an expert backend engineer specializing in production-grade Go microservices.

Generate a clean, production-ready Golang microservice with the following strict engineering standards and best practices.

General Requirements:
- Use Go modules
- Follow clean architecture / layered architecture
- Structure code in packages such as:
  cmd/
  internal/
  config/
  handlers/
  services/
  repository/
  models/
  middleware/
  utils/
- Separate business logic from HTTP layer
- Follow SOLID principles
- Avoid code duplication
- Use dependency injection where appropriate
- Avoid global variables

Coding Standards:
- Never hardcode strings, numbers, or configuration values
- Use constants for static values
- Use environment variables for configuration
- All configuration must come from config files or env variables
- Use struct tags for JSON serialization
- Use meaningful variable and function names
- Follow Go naming conventions (camelCase for private, PascalCase for exported)

Error Handling:
- Never ignore errors
- Use wrapped errors with context
- Return structured errors from services
- Log errors properly
- Do not use panic in production code

Logging:
- Use structured logging
- Use a logging library like zap or logrus
- Include request ID and context in logs

Configuration:
- Load configuration from environment variables
- Use a config package
- Support different environments (dev, staging, prod)

HTTP Layer:
- Use a router like chi or gin
- Implement request validation
- Return proper HTTP status codes
- Use middleware for logging, recovery, and authentication
- Add request timeouts

Database:
- Use a repository layer
- Use context.Context for DB operations
- Avoid direct SQL inside handlers
- Use connection pooling

Concurrency:
- Ensure thread safety
- Use goroutines responsibly
- Avoid race conditions
- Use context for cancellation

API Design:
- Follow RESTful conventions
- Version APIs (e.g., /v1/users)
- Use consistent response formats

Testing:
- Include unit tests
- Mock dependencies
- Test handlers, services, and repositories

Security:
- Validate all inputs
- Avoid exposing sensitive data
- Use secure headers
- Implement rate limiting middleware

Performance:
- Avoid unnecessary allocations
- Use connection reuse
- Implement graceful shutdown
- Add health check endpoints

Observability:
- Add /health endpoint
- Add metrics endpoint
- Support distributed tracing if possible

The generated code should be clean, modular, production-ready, and easily extensible.
