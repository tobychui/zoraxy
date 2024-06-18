# Example www Folder

This is an example www folder that contains two sub-folders.

- `html/`
- `templates/`

The html file contain static resources that will be served by Zoraxy build-in static web server. You can use it as a generic web server with a static site generator like [Hugo](https://gohugo.io/) or use it as a small CDN for serving your scripts / image that commonly use across many of your sites.

The templates folder contains the template for overriding the build in error or access denied pages. The following templates are supported

- notfound.html (Default site Not-Found error page)
- whitelist.html (Error page when client being blocked by whitelist rule)
- blacklist.html (Error page when client being blocked by blacklist rule)

To use the template, copy and paste the `wwww` folder to the same directory as zoraxy executable (aka the src/ file if you `go build` with the current folder tree). 



### Other Templates

There are a few pre-built templates that works with Zoraxy where you can find in the `other-templates` folder. Copy the folder into `www` and rename the folder to `templates` to active them. 



It is worth mentioning that the uwu icons for not-found and access-denied are created by @SAWARATSUKI
