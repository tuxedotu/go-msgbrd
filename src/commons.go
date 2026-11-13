package main

import (
	"sync"
	"time"
)

// data structures
type User struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	CreatedAt time.Time
}

type Message struct {
	Id        int
	Author    string
	Msg       string
	CreatedAt time.Time
}

type session struct {
	userId    int
	expiry    time.Time
	createdAt time.Time
}

// global variables -> TODO: convert to a lightweight sqlite setup potentially? 
var userCache = make(map[int]User)
var sessionsCache = make(map[string]session)
var messageCache = make(map[int]Message)
var cacheMutex sync.RWMutex
