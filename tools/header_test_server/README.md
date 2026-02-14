# HTTP/1.1 Test Server

A simple HTTP test server that only accepts HTTP/1.1 connections and displays detailed request information.

## Features

- **HTTP/1.1 Enforcement**: Rejects any connections not using HTTP/1.1 protocol (returns 505 HTTP Version Not Supported)
- **Comprehensive Request Information Display**:
  - Protocol version details (major/minor)
  - Request method, URL, path, and query string
  - Connection information (remote IP, port, IP type detection)
  - All request headers (sorted alphabetically)
  - Connection attributes (content length, transfer encoding, keep-alive)
  - TLS information (when using HTTPS)
  - HTTP/1.1 supported features list
  - Query parameters parsing
  - User-Agent and Referer information

## Usage

### Running the Server

```bash
go run main.go
```

The server will start on port `8080` by default.

### Testing the Server

#### Basic HTTP/1.1 Request
```bash
curl http://localhost:8080
```

#### With Custom Headers
```bash
curl -H "X-Custom-Header: test-value" http://localhost:8080/test?param=value
```

#### Force HTTP/1.1
```bash
curl --http1.1 http://localhost:8080
```

#### Test HTTP/2 Rejection (requires HTTPS)
```bash
curl --http2 https://localhost:8443
# Will receive: Error: Only HTTP/1.1 is supported
```

### Expected Output

The server returns a plain text response with detailed request information:

```
========================================
HTTP/1.1 TEST SERVER - REQUEST INFO
========================================

PROTOCOL INFORMATION:
  Protocol: HTTP/1.1
  Major Version: 1
  Minor Version: 1

REQUEST LINE:
  Method: GET
  URL: /test?param=value
  Path: /test
  RawQuery: param=value

CONNECTION INFORMATION:
  Remote Address: 127.0.0.1:54321
  Remote IP: 127.0.0.1
  Remote Port: 54321
  IP Type: IPv4
  Host: localhost:8080

REQUEST HEADERS:
  Accept: */*
  User-Agent: curl/7.68.0
  ...

[Additional information sections]
```

## How It Works

1. The server listens on port 8080 using Go's standard `http.Server`
2. Each incoming request is checked for protocol version in the handler
3. If the request is not HTTP/1.1 (e.g., HTTP/2, HTTP/0.9), it's rejected with a 505 status code
4. Valid HTTP/1.1 requests receive a detailed response showing all request attributes
5. All requests (accepted and rejected) are logged to the console

## Use Cases

- Testing HTTP/1.1 client implementations
- Debugging HTTP request headers and connection properties
- Verifying protocol version compatibility
- Educational purposes for understanding HTTP/1.1 protocol
- Integration testing for applications requiring HTTP/1.1

## Configuration

To change the port, modify the `Addr` field in `main.go`:

```go
server := &http.Server{
    Addr:    ":8080",  // Change port here
    Handler: http.HandlerFunc(handleRequest),
}
```

## Notes

- The server automatically detects IPv4 vs IPv6 connections
- TLS information is displayed when using HTTPS
- Headers are sorted alphabetically for consistent output
- All request processing happens after HTTP protocol negotiation is complete
