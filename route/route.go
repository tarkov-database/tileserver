package route

import (
	"net/http"

	cntrl "github.com/tarkov-database/tileserver/controller"

	"github.com/julienschmidt/httprouter"
)

const prefix = "/v1"

// Load returns a router with defined routes
func Load() *httprouter.Router {
	return routes()
}

func routes() *httprouter.Router {
	r := httprouter.New()

	// Index
	// r.GET(prefix, cntrl.IndexGET)
	r.Handler("GET", "/", http.RedirectHandler(prefix, http.StatusMovedPermanently))

	// Tileset
	r.GET(prefix+"/:id", cntrl.TileJSONGET)
	r.GET(prefix+"/:id/tiles/:z/:x/:y", cntrl.TileGET)

	r.RedirectTrailingSlash = true
	r.HandleOPTIONS = true

	return r
}
