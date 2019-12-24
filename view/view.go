package view

import (
	"encoding/json"
	"net/http"

	"github.com/tarkov-database/tileserver/core/mbtiles"
	"github.com/tarkov-database/tileserver/model"

	"github.com/google/logger"
)

const contentTypeJSON = "application/json"

// RenderJSON encodes the input data into JSON and sends it as response
func RenderJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(&data); err != nil {
		logger.Error(err)
	}
}

func Tile(w http.ResponseWriter, t *model.Tile, status int) {
	w.Header().Set("Content-Type", t.Format.ContentType())
	if t.Format == mbtiles.PBF {
		w.Header().Set("Content-Encoding", "gzip")
	}
	w.WriteHeader(status)

	w.Write(t.Data)
}

func Grid(w http.ResponseWriter, t *model.Tile, status int) {
	w.Header().Set("Content-Type", contentTypeJSON)
	if t.Format == mbtiles.ZLIB {
		w.Header().Set("Content-Encoding", "deflate")
	} else {
		w.Header().Set("Content-Encoding", "gzip")
	}
	w.WriteHeader(status)

	w.Write(t.Data)
}
