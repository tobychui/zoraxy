# Introduction
A minimalist proxy library for go, inspired by `net/http/httputil` and add support for HTTPS using HTTP Tunnel

Support cancels an in-flight request by closing it's connection

# Installation
```sh
go get github.com/cssivision/reverseproxy
```

# Usage

## A simple proxy
```go
package main

import (
    "net/http"
    "net/url"
    "github.com/cssivision/reverseproxy"
)

func main() {
    http.ListenAndServe(":8080", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        path, err := url.Parse("https://github.com")
        if err != nil {
            panic(err)
            return
        }
        proxy := reverseproxy.NewReverseProxy(path)
        proxy.ServeHTTP(w, r)
    }))
}
```

## Use as a proxy server

To use proxy server, you should set browser to use the proxy server as an HTTP proxy.

```go
package main

import (
    "net/http"
    "net/url"
    "github.com/cssivision/reverseproxy"
)

func main() {
    http.ListenAndServe(":8080", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        path, err := url.Parse("http://" + r.Host)
        if err != nil {
            panic(err)
            return
        }

        proxy := reverseproxy.NewReverseProxy(path)
        proxy.ServeHTTP(w, r)

        // Specific for HTTP and HTTPS
        // if r.Method == "CONNECT" {
        //     proxy.ProxyHTTPS(w, r)
        // } else {
        //     proxy.ProxyHTTP(w, r)
        // }
    }))
}
```
