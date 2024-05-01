# Example www Folder

This is an example www folder that contains two sub-folders.

- `html/`
- `templates/`

The html file contain static resources that will be served by Zoraxy build-in static web server. You can use it as a generic web server with a static site generator like [Hugo](https://gohugo.io/) or use it as a small CDN for serving your scripts / image that commonly use across many of your sites.

The templates folder contains the template for overriding the build in error or access denied pages. The following templates are supported

- notfound.html (Default site Not-Found error page)
- whitelist.html (Error page when client being blocked by whitelist rule)
- blacklist.html (Error page when client being blocked by blacklist rule)



To use the template, copy and paste the `wwww` folder to the same directory as zoraxy executable (aka the src/ file if you `go build` with the current folder tree) . 