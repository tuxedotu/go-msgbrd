# go-msgbrd
a cute little web-messageboard written via go + htmx templates


The initial codebase was migrated over from my 'learning-go/websrv' repository due to the an initial exploration of Go web-architecture growing in complexity and kinda morphing into it's own little project.  

Just clone the repo and run the messageboard via `go run ./src` inside the project folder to tinker around yourself :)

## Current Todo's
- [x] implement Server-Sent Events (SSEs) for on-demand msg-reload via push from the server 
    - Go's Channel-Architecture seems very convenient and perfect for this use-case (see use of `messageChannels` in codebase)
