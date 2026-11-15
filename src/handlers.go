package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os/exec"
	"slices"
	"time"
)

// TODO: unify login / signup logic
func createUser(writer http.ResponseWriter, req *http.Request) {
	var user User
	var sessionToken string

	// ADD NEW USER OBJECT //
	user = User{Name: req.FormValue("name"), CreatedAt: time.Now()} // NOTE: potentially unsafe? check ramifications of bogus / malicious raw user form-input)
	if user.Name == "" {
		fmt.Println("- ERR in 'createUser': Name required!")
		http.Error(writer, "Name required!", http.StatusBadRequest)
		return
	}
	cacheMutex.Lock()
	for _, tmpUser := range userCache {
		if tmpUser.Name == user.Name {
			cacheMutex.Unlock()
			fmt.Println("- (creatUsr) ERR: Username exists!")
			templ := template.Must(template.ParseFiles("./www/legos/error.html"))
			templ.ExecuteTemplate(writer, "error", "Username already exists!")
			return
		}
	}
	user.Id = len(userCache) + 1
	userCache[user.Id] = user
	cacheMutex.Unlock()
	fmt.Printf("- POST user (OK): %v\n", user)

	// CREATE NEW SESSION-TOKEN based on new user & add cookie //
	cmdOut, err := exec.Command("uuidgen").Output()
	if err != nil {
		fmt.Printf("- ERR: %v\n", err)
		return
	}
	sessionToken = string(cmdOut)[0 : len(string(cmdOut))-1] //>> byte-array to clean string conversion (w/o '\n')
	sessionsCache[sessionToken] = session{
		userId:    user.Id,
		expiry:    user.CreatedAt.Add(10 * time.Minute),
		createdAt: time.Now(),
	}
	http.SetCookie(writer, &http.Cookie{
		Name:    "session_token",
		Value:   sessionToken,
		Expires: sessionsCache[sessionToken].expiry,
	})
	fmt.Printf("- session created: %v >> %T\n", sessionToken, sessionsCache[sessionToken])

	/// write response ///
	writer.Header().Set("HX-Redirect", "/") //>> *magic* HTMX-Page Redirects
	writer.WriteHeader(http.StatusOK)
}

func loginUser(w http.ResponseWriter, r *http.Request) {
	var userInput string
	var familiarUser User

	userInput = r.FormValue("name") //> unsanitary user-input ??
	for _, user := range userCache {
		if user.Name == userInput {
			familiarUser = user
		}
	}

	if familiarUser.Name == "" {
		// user does not exist ? create one..
		createUser(w, r)
		return
	}

	for sessionToken, session := range sessionsCache {
		if session.userId == familiarUser.Id && time.Now().After(session.expiry) {
			session.expiry = time.Now().Add(10 * time.Minute)
			sessionsCache[sessionToken] = session
			http.SetCookie(w, &http.Cookie{
				Name:    "session_token",
				Value:   sessionToken,
				Expires: session.expiry,
			})
		}
	}
	if w.Header().Get("session_token") == "" {
		cmdOut, err := exec.Command("uuidgen").Output()
		if err != nil {
			fmt.Printf("- ERR: couldn't create sessionToken!")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		sessionToken := string(cmdOut)[:len(string(cmdOut))-1] //>> byte-array to clean string conversion (w/o '\n')
		newSession := session{
			userId:    familiarUser.Id,
			expiry:    time.Now().Add(10 * time.Minute),
			createdAt: time.Now(),
		}
		sessionsCache[string(sessionToken)] = newSession
		http.SetCookie(w, &http.Cookie{
			Name:    "session_token",
			Value:   string(sessionToken),
			Expires: newSession.expiry,
		})
	}

	fmt.Printf("- (login) LOG: Success! input = '%v' (u: %v)\n", userInput, familiarUser.Name)
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

func updateUser(w http.ResponseWriter, r *http.Request) {
	var userInput string
	var user User

	userInput = r.FormValue("name")
	if userInput == "" {
		fmt.Printf("- ERR: no empty user-updates allowed!\n")
		http.Error(w, "No empty updates allowed!", http.StatusBadRequest)
		return
	}
	session, err := authenticateRequest(r)
	if err != nil {
		fmt.Printf("- ERR: %v\n", err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	cacheMutex.Lock()
	for _, currUser := range userCache {
		if currUser.Name == userInput {
			cacheMutex.Unlock()
			fmt.Printf("- ERR: User already exists!\n")
			templ := template.Must(template.ParseFiles("./www/legos/error.html"))
			templ.ExecuteTemplate(w, "error", "User already exists!")
			return
		}
	}
	user = userCache[session.userId]
	user.Name = userInput
	userCache[session.userId] = user
	cacheMutex.Unlock()

	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

func userPostsMessage(w http.ResponseWriter, r *http.Request) {
	var userInput string
	var postedMessage Message
	var templ *template.Template
	var templData = struct {
		User     User
		Messages map[int]Message
	}{}

	userInput = r.FormValue("message")
	if userInput == "" {
		fmt.Printf("- ERR: No empty messages allowed!\n")
		http.Error(w, "No empty messages allowed!", http.StatusBadRequest)
		return
	}

	session, err := authenticateRequest(r)
	if err != nil {
		fmt.Printf("- ERR: %v\n", err)
		templ = template.Must(templ.ParseFiles("./www/legos/error.html"))
		templ.ExecuteTemplate(w, "error", "Your session has expired (just 'refresh' or 'login' again ^^)")
		return
	}

	cacheMutex.Lock()
	postedMessage = Message{
		Id:        len(messageCache) + 1,
		Author:    userCache[session.userId].Name,
		Msg:       userInput,
		CreatedAt: time.Now(),
	}
	messageCache[postedMessage.Id] = postedMessage
	templData.User = userCache[session.userId]
	templData.Messages = messageCache

	// send posted message as json to all currently connected / active channels 
	if msgJson, err := json.Marshal(postedMessage); err == nil {
		for _, c := range messageChannels {
			c <- string(msgJson)
		}
	}
	cacheMutex.Unlock()
	fmt.Printf("- (postMsg) From: '%v' (u: '%v')\n\t++ postedMessage = '%v'\n", r.RemoteAddr, postedMessage.Author, postedMessage.Msg)
	fmt.Printf("- (msg-stream) Sending new message to %d connected clients.\n", len(messageChannels))
}

func refreshMessages(w http.ResponseWriter, r *http.Request) {
	var tmpl *template.Template
	tmpl = template.Must(tmpl.ParseFiles("./www/legos/messageboard.html"))
	tmpl.ExecuteTemplate(w, "messages", messageCache)
}

// Server-Sent Events -> on-demand message broadcasting instead of client-side hot pinging
func serverSentMessagePing(w http.ResponseWriter, r *http.Request) {
	// create new message channel for newly connecting client
	var msgChannel = make(chan string)
	cacheMutex.Lock()
	messageChannels = append(messageChannels, msgChannel)
	cacheMutex.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// forever loop until client disconnects
	if flusher, ok := w.(http.Flusher); ok {
		fmt.Printf("- (msg-stream) Client: '%v' listening for messages...\n", r.RemoteAddr)

		for {
			select {
				case <-r.Context().Done():
					// on disconnect -> close channel and remove from listener-list 
					cacheMutex.Lock()
					var channelIndex = slices.Index(messageChannels, msgChannel)
					messageChannels = slices.Delete(messageChannels, channelIndex, channelIndex+1)
					cacheMutex.Unlock()
					close(msgChannel)
					fmt.Printf("- (msg-stream) Client '%v' closed his message channel.\n", r.RemoteAddr)
					return
				default:
					msg := <-msgChannel // blocks until new message is available from channel
					fmt.Fprintf(w, "data: %s\n\n", msg)
					flusher.Flush()
			}
		}
	} else {
		fmt.Printf("- (msg-stream) Client: '%v' couldn't open message stream!\n\n", r.RemoteAddr)
	}
}
