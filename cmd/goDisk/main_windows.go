package main

import (
	"context"
	"fmt"
	"os"

	"github.com/egoist/mygo"
	"goDisk/internal/application"
	"goDisk/internal/storage"
	desktop "goDisk/ui/mygo"
)

func main() {
	store, storeErr := storage.Open()
	if store != nil {
		defer store.Close()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mygo.App.WhenReady(func() {
		desktop.Open(ctx, application.App{Store: store}, storeErr)
	})
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{Title: "goDisk", Type: mygo.MessageError, Message: "无法启动 goDisk", Detail: err.Error()})
		os.Exit(1)
	}
}
