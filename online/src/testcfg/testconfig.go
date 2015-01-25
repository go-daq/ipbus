package main

import (
    "config"
    "fmt"
    "os"
)

func main() {
    dir, _ := os.Getwd()
    fmt.Printf("Working at %s\n", dir)
    //g := config.NewGLIB(6, "align_GLIB6.json", "pedspa_GLIB6.json", "masks_GLIB6.json")
    g := config.Load(6)
    fmt.Printf("GLIB%d:\n", g.Module)
    for _, ch := range g.DataChannels {
        fmt.Printf("    %v\n", ch)
    }
}
