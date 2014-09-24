package main

import (
    "fmt"
    "solid"
    "net"
    "time"
)

func main() {
    con := solid.New()
    for i := 0; i < 5; i++ {
        loc := fmt.Sprintf("localhost:%d", 9988 + i)
        addr, err := net.ResolveUDPAddr("udp", loc)
        if err != nil {
            panic(err)
        }
        con.AddFPGA(addr)
    }
    con.Start()
    dt := 10 * time.Second
    fmt.Printf("Going to run for %v.\n", dt)
    con.Run("test", dt)
    con.Quit()
}
