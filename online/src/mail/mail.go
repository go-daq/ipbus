package mail
import (
    "data"
    "fmt"
    "net/smtp"
    "os"
    "strings"
    "time"
)

var to = []string{"nick.ryder@physics.ox.ac.uk"}

type E struct {
    pass string
}

func (e *E) Load(fn string) error {
    inp, err := os.Open(fn)
    if err != nil {
        return err
    }
    defer inp.Close()
    buf := make([]byte, 1024)
    n, err := inp.Read(buf)
    if err != nil {
        return err
    }
    e.pass = string(buf[:n])
    e.pass = strings.Replace(e.pass, "\n", "", -1)
    return error(nil)
}

func (e E) Send(subject, msg string) error {
    if e.pass == "" {
        return fmt.Errorf("Sending email: password not loaded.")
    }
    auth := smtp.PlainAuth("", "solid.daq@gmail.com", e.pass, "smtp.gmail.com")
    out := []byte(fmt.Sprintf("Subject: %s\n%s", subject, msg))
    err := smtp.SendMail("smtp.gmail.com:587", auth, "solid.daq@gmail.com", to, out)
    return err
}

func (e E) Log(msg string, errp data.ErrPack) error {
    subject := fmt.Sprintf("Online DAQ error at %v", time.Now())
    out := fmt.Sprintf("%s\n%v\n", msg, errp)
    err := e.Send(subject, out)
    return err
}

func (e E) Handle(ep data.ErrPack, msg string) {
    subject := fmt.Sprintf("Online DAQ crash at %v", time.Now())
    emsg := fmt.Sprintf("Caught a panic: %s, %v\n", msg, ep)
    e.Send(subject, emsg)
}
