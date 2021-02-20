package main

import (
	"fmt"
	"io"
	"os"

	"github.com/tarkov-database/tileserver/core/mbtiles"
	"github.com/tarkov-database/tileserver/core/server"
	"github.com/tarkov-database/tileserver/model"

	"github.com/google/logger"
)

func main() {
	fmt.Printf("Starting up Tarkov Database TileServer\n\n")

	defLog := logger.Init("default", true, false, io.Discard)
	defer defLog.Close()

	tsDir := "./tilesets"
	if env := os.Getenv("TILE_DIR"); len(env) > 0 {
		tsDir = env
	}

	if err := mbtiles.LoadTilesets(tsDir); err != nil {
		logger.Errorf("Tileset loading error: %v", err)
		model.SetInitAsFailed()
	}

	if err := server.ListenAndServe(); err != nil {
		logger.Errorf("HTTP server error: %s", err)
	}
}
