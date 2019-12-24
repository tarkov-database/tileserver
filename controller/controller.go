package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

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

func TileGET(w http.ResponseWriter, _ *http.Request, ps httprouter.Params) {
	id := ps.ByName("id")
	z, x, y := ps.ByName("z"), ps.ByName("x"), ps.ByName("y")

	isGrid := strings.HasSuffix(ps.ByName("y"), ".json")

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

	if isGrid {
		view.Grid(w, tile, http.StatusOK)
	} else {
		view.Tile(w, tile, http.StatusOK)
	}
}
