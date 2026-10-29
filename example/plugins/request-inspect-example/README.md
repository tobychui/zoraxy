# Request Inspection Example

This example requires the `inspect-hook` Zoraxy branch. It does **not** capture or forward requests: `?block=1` returns `deny`; otherwise `allow` continues the usual Zoraxy routing chain.

Build/test within this directory:

```sh
go test ./...
go build -o /path/to/isolated/plugins/request-inspect-example/request-inspect-example .
```

The binary supports Zoraxy's `-introspect` and `-configure=<JSON>` protocol and binds only `127.0.0.1:<assigned port>`. Enable it in the test instance, add it to a plugin tag/group, then assign that tag to the intended HTTP host route. A nonempty UIPath is retained because the existing plugin-spec validator requires it.

`POST /inspect` receives the existing sniff metadata flattened into JSON, plus `body` (base64, empty string for no body) and `body_truncated` (boolean). HTTP 200 responses contain `{"action":"allow"}` or `{"action":"deny","status":403,"reason":"Blocked by request-inspect-example"}`. This is a query demonstration, **not** a WAF.

The SDK copies here match the changed core copies; pre-existing example SDKs are deliberately untouched. No new dependency on the core Zoraxy module.
