package controller

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/tarkov-database/tileserver/core/mbtiles"
	"github.com/tarkov-database/tileserver/model"
	"github.com/tarkov-database/tileserver/view"

	"github.com/julienschmidt/httprouter"
)

var host *url.URL

func init() {
	if env := os.Getenv("HOST_URL"); len(env) > 0 {
		var err error
		host, err = url.Parse(env)
		if err != nil {
			fmt.Fprintln(os.Stderr, fmt.Sprintf("Error while parsing HOST_URL environment variable: %s", err))
			os.Exit(2)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Host URL not set!")
		os.Exit(2)
	}
}

func TileJSONGET(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	r.URL.Scheme, r.URL.Host = host.Scheme, host.Host

	tj, err := model.GetTileJSON(ps.ByName("id"), r.URL)
	if err != nil {
		res := model.NewResponse("Tileset not found", http.StatusNotFound)
		view.RenderJSON(w, res, res.StatusCode)
		return
	}

	view.RenderJSON(w, tj, http.StatusOK)
}

func TileGET(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	var id, z, x, y string

	for _, v := range ps {
		switch v.Key {
		case "id":
			id = v.Value
		case "z":
			z = v.Value
		case "x":
			x = v.Value
		case "y":
			y = v.Value
		}
	}

	isGrid := strings.HasSuffix(y, ".json")

	var err error
	var tile *model.Tile

	if isGrid {
		tile, err = model.GetGrid(id, z, x, y)
	} else {
		tile, err = model.GetTile(id, z, x, y)
	}

	if err != nil {
		switch {
		case errors.Is(err, mbtiles.ErrTilesetNotFound):
			http.Error(w, "Tileset not found", http.StatusNotFound)
		case errors.Is(err, mbtiles.ErrTileNotFound), errors.Is(err, mbtiles.ErrNoUTFGrid):
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, mbtiles.ErrInvalidTileCoord):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	if header := r.Header.Get("If-Modified-Since"); len(header) > 0 {
		since, err := time.Parse(http.TimeFormat, header)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if !tile.Modified.After(since) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	hash := hex.EncodeToString(tile.Hash[:])
	if r.Header.Get("If-None-Match") == hash {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Last-Modified", tile.Modified.Format(http.TimeFormat))
	w.Header().Set("ETag", hash)

	if isGrid {
		view.Grid(w, tile, http.StatusOK)
	} else {
		view.Tile(w, tile, http.StatusOK)
	}
}
