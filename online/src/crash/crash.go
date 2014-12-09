package crash

import (
    "data"
    "fmt"
    "os"
    "time"
)

type Handler interface {
    Handle(data.ErrPack)
}

type Exit struct {
    handlers []Handler
}

func (e *Exit) Add(h Handler) {
    e.handlers = append(e.handlers, h)
}

func (e *Exit) CleanExit() {
    panicerr := error(nil)
    if r := recover(); r != nil {
        if err, ok := r.(error); ok {
            panicerr = err
        }
    }
    ep := data.MakeErrPack(panicerr)
    for _, h := range e.handlers {
        h.Handle(ep)
    }
    fmt.Println("Crash detected and cleaned. Exiting...")
    os.Exit(1)
}

type Log struct {
}

func (l Log) Handle(ep data.ErrPack) {
    msg := fmt.Sprintf("Caught a panic: %v\n", ep)
    fmt.Println(msg)
    layout := "2Jan2006_15hr04"
    now := time.Now()
    fn := fmt.Sprintf("panic_%s.log", now.Format(layout))
    if outp, err := os.Create(fn); err == nil {
        outp.WriteString(msg)
        outp.Close()
    }
}
