// Command seed populates a running Discurd stack with demo data by driving
// the public REST API. It is idempotent: re-running logs the demo users in
// instead of re-registering them. Uses only the standard library.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type seedUser struct {
	Username string
	Email    string
	Token    string
	ID       string
}

type client struct {
	base string
}

// do sends a JSON request and decodes the response into out (if non-nil).
// It returns the HTTP status.
func (c *client) do(method, path, token string, body, out any) (int, error) {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return 0, err
		}
	}
	req, err := http.NewRequest(method, c.base+path, &buf)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var apiErr apiError
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		return resp.StatusCode, fmt.Errorf("%s %s -> %d %s: %s",
			method, path, resp.StatusCode, apiErr.Error.Code, apiErr.Error.Message)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("%s %s: decode response: %w", method, path, err)
		}
	}
	return resp.StatusCode, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "seed: "+format+"\n", args...)
	os.Exit(1)
}

func (c *client) waitReady() {
	deadline := time.Now().Add(90 * time.Second)
	for {
		resp, err := httpClient.Get(c.base + "/readyz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			fatalf("api at %s never became ready", c.base)
		}
		fmt.Println("waiting for api to become ready...")
		time.Sleep(2 * time.Second)
	}
}

// authenticate registers the user, falling back to login on conflict.
func (c *client) authenticate(u *seedUser, password string) {
	var authResp struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		AccessToken string `json:"access_token"`
	}
	status, err := c.do(http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"username": u.Username,
		"email":    u.Email,
		"password": password,
	}, &authResp)
	if status == http.StatusConflict {
		if _, err := c.do(http.MethodPost, "/api/v1/auth/login", "", map[string]string{
			"email":    u.Email,
			"password": password,
		}, &authResp); err != nil {
			fatalf("login %s: %v", u.Username, err)
		}
		fmt.Printf("logged in existing user %s\n", u.Username)
	} else if err != nil {
		fatalf("register %s: %v", u.Username, err)
	} else {
		fmt.Printf("registered user %s\n", u.Username)
	}
	u.Token = authResp.AccessToken
	u.ID = authResp.User.ID
}

type channelObj struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func main() {
	base := os.Getenv("API_URL")
	if base == "" {
		base = "http://api:8080"
	}
	c := &client{base: base}
	fmt.Printf("seeding Discurd via %s\n", base)
	c.waitReady()

	const password = "password123"
	alice := &seedUser{Username: "alice", Email: "alice@discurd.dev"}
	bob := &seedUser{Username: "bob", Email: "bob@discurd.dev"}
	charlie := &seedUser{Username: "charlie", Email: "charlie@discurd.dev"}
	for _, u := range []*seedUser{alice, bob, charlie} {
		c.authenticate(u, password)
	}

	// Guild: reuse "Discurd HQ" if alice already has it (re-runnable).
	var guild struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	var myGuilds []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if _, err := c.do(http.MethodGet, "/api/v1/users/@me/guilds", alice.Token, nil, &myGuilds); err != nil {
		fatalf("list alice's guilds: %v", err)
	}
	for _, g := range myGuilds {
		if g.Name == "Discurd HQ" {
			guild.ID, guild.Name = g.ID, g.Name
			break
		}
	}
	if guild.ID == "" {
		if _, err := c.do(http.MethodPost, "/api/v1/guilds", alice.Token,
			map[string]string{"name": "Discurd HQ"}, &guild); err != nil {
			fatalf("create guild: %v", err)
		}
		fmt.Printf("created guild %q (%s)\n", guild.Name, guild.ID)
	} else {
		fmt.Printf("reusing guild %q (%s)\n", guild.Name, guild.ID)
	}

	// Channels: #general is auto-created; add #random and #dev.
	var channels []channelObj
	if _, err := c.do(http.MethodGet, "/api/v1/guilds/"+guild.ID+"/channels", alice.Token, nil, &channels); err != nil {
		fatalf("list channels: %v", err)
	}
	byName := map[string]channelObj{}
	for _, ch := range channels {
		byName[ch.Name] = ch
	}
	for _, spec := range []struct{ name, topic string }{
		{"random", "Anything goes"},
		{"dev", "Building Discurd itself"},
	} {
		if _, ok := byName[spec.name]; ok {
			continue
		}
		var ch channelObj
		if _, err := c.do(http.MethodPost, "/api/v1/guilds/"+guild.ID+"/channels", alice.Token,
			map[string]string{"name": spec.name, "topic": spec.topic}, &ch); err != nil {
			fatalf("create channel #%s: %v", spec.name, err)
		}
		byName[ch.Name] = ch
		fmt.Printf("created channel #%s\n", ch.Name)
	}
	general, ok := byName["general"]
	if !ok {
		fatalf("guild has no #general channel")
	}
	random := byName["random"]
	dev := byName["dev"]

	// Invite + joins (idempotent).
	var invite struct {
		Code string `json:"code"`
	}
	if _, err := c.do(http.MethodPost, "/api/v1/guilds/"+guild.ID+"/invites", alice.Token, nil, &invite); err != nil {
		fatalf("create invite: %v", err)
	}
	for _, u := range []*seedUser{bob, charlie} {
		if _, err := c.do(http.MethodPost, "/api/v1/invites/"+invite.Code+"/join", u.Token, nil, nil); err != nil {
			fatalf("%s join invite: %v", u.Username, err)
		}
		fmt.Printf("%s joined %q\n", u.Username, guild.Name)
	}

	// ~40 varied messages across the three channels from all three users.
	type post struct {
		channel channelObj
		author  *seedUser
		content string
	}
	posts := []post{
		{general, alice, "Welcome to Discurd HQ, everyone! :tada:"},
		{general, bob, "hey hey! good to be here"},
		{general, charlie, "o/ hello from charlie"},
		{general, alice, "House rules: be kind, ship code, use #dev for build talk."},
		{general, bob, "sounds good. does this thing have typing indicators?"},
		{general, charlie, "it does, I can see you typing right now :eyes:"},
		{general, alice, "Presence dots too — green means online."},
		{general, bob, "slick. the message pane scrolls up forever?"},
		{general, alice, "Infinite upward scroll, 10-day buckets under the hood."},
		{general, charlie, "same trick Discord uses with Scylla partitions, right?"},
		{general, alice, "Exactly — (channel_id, bucket) partitions, timeuuid clustering."},
		{general, bob, "nerds. I love it here already"},
		{general, charlie, "someone pin this conversation"},
		{general, alice, "Pins are on the roadmap ;)"},
		{random, bob, "obligatory first post in #random: 🦆"},
		{random, charlie, "is that a duck or a very committed goose"},
		{random, bob, "it's whatever the content_type says it is"},
		{random, alice, "attachment uploads work btw, drop an image here"},
		{random, charlie, "hot take: tabs > spaces"},
		{random, bob, "blocked and reported"},
		{random, alice, "gofmt has entered the chat"},
		{random, charlie, "fine, fine, the formatter wins as always"},
		{random, bob, "lunch poll: tacos or ramen?"},
		{random, alice, "ramen. it's not even close."},
		{random, charlie, "tacos, obviously. democracy is broken"},
		{random, bob, "2-1 ramen wins, charlie owes us tacos anyway"},
		{random, alice, "this channel is living up to its topic"},
		{dev, alice, "Status: api + gateway are up, seed script is running (hi, that's me)."},
		{dev, bob, "nice. websocket fan-out is through NATS subjects?"},
		{dev, alice, "Yep — discurd.events.guild.{id} and discurd.events.user.{id}."},
		{dev, charlie, "how do gateways know which sockets get a guild event?"},
		{dev, alice, "Each session caches its user's guild-id set at identify time."},
		{dev, bob, "and rate limits? I refuse to be throttled"},
		{dev, alice, "120 req/min global, 10 messages per 10s per channel. You will be throttled."},
		{dev, charlie, "presence is redis keys with a 70s TTL, heartbeat refreshes them"},
		{dev, bob, "metrics look good in grafana, ws connection gauge is climbing"},
		{dev, alice, "Don't forget /readyz checks scylla, redis, and nats."},
		{dev, charlie, "edited messages get an edited_at timestamp, just tested it"},
		{dev, bob, "deleting my hot takes from #random as we speak"},
		{dev, alice, "Ship it. :rocket:"},
	}

	posted := 0
	for _, p := range posts {
		if _, err := c.do(http.MethodPost, "/api/v1/channels/"+p.channel.ID+"/messages", p.author.Token,
			map[string]any{"content": p.content}, nil); err != nil {
			fatalf("post message to #%s as %s: %v", p.channel.Name, p.author.Username, err)
		}
		posted++
	}

	fmt.Println()
	fmt.Println("=== seed complete ===")
	fmt.Printf("guild:    %s (%s)\n", guild.Name, guild.ID)
	fmt.Printf("channels: #general %s, #random %s, #dev %s\n", general.ID, random.ID, dev.ID)
	fmt.Printf("users:    alice, bob, charlie (password: %s)\n", password)
	fmt.Printf("messages: %d posted\n", posted)
	fmt.Printf("invite:   %s  (join at /invites/%s/join)\n", invite.Code, invite.Code)
}
