package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "net/http"
)

func New(boardid, auth string) HipChat {
    c := &http.Client{}
    buf := new(bytes.Buffer)
    url := fmt.Sprintf("https://api.hipchat.com/v2/room/%s/notification", boardid)
    auth = fmt.Sprintf("Bearer %s", auth)
    return HipChat{auth, c, buf, url}
}

type HipChat struct {
    auth string
    client *http.Client
    buf *bytes.Buffer
    notifyurl string
}

func (hc HipChat) send(n Notification) error {
    if err := json.NewEncoder(hc.buf).Encode(n); err != nil {
        panic(err)
    }
    req, err := http.NewRequest("POST", hc.notifyurl, hc.buf)
    if err != nil {
        panic(err)
    }
    req.Header.Add("Authorization", hc.auth)
    req.Header.Add("Content-Type", "application/json")
    resp, err := hc.client.Do(req)
    defer resp.Body.Close()
    if err != nil {
        panic(err)
    }
    return nil
}

func (hc HipChat) Warning(msg string) error {
    return hc.send(Notify(msg, "yellow", true))
}

func (hc HipChat) Error(msg string) error {
    return hc.send(Notify(msg, "red", true))

}

func (hc HipChat) Status(msg string) error {
    return hc.send(Notify(msg, "green", false))
}

type Notification struct {
    Colour string `json:"color"`
    Message string `json:"message"`
    Format string `json:"message_format"`
    Notify bool `json:"notify"`
}

func Notify (msg, colour string, notify bool) Notification {
    return Notification{colour, msg, "text", notify}
}


func main() {
    token, err := ioutil.ReadFile("token.txt")
    if err != nil {
        panic(err)
    }
    roomid := "979750"
    hc := New(roomid, string(token))
    if err := hc.Status("Everything is good."); err != nil {
        panic(err)
    }
    if err := hc.Warning("Actually, there might be a problem..."); err != nil {
        panic(err)
    }
    if err := hc.Error("Holy crap, @here, @NickRyder, you better read this."); err != nil {
        panic(err)
    }
}
