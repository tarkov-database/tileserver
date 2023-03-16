package model

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/tarkov-database/tileserver/core/mbtiles"

	"github.com/zeebo/blake3"
)

var (
	ErrNoEntity = errors.New("entity does not exist")
	ErrBadInput = errors.New("invalid input")
)

const (
	tileJSONVersion = "2.2.0"
	tileJSONScheme  = "xyz"
)

// TileJSON describes a tileset in JSON format
type TileJSON struct {
	// TileJSON spec fields
	TileJSON    string     `json:"tilejson"`
	Name        string     `json:"name,omitempty"`
	Description string     `json:"description,omitempty"`
	Version     string     `json:"version,omitempty"`
	Attribution string     `json:"attribution,omitempty"`
	Template    string     `json:"template,omitempty"`
	Legend      string     `json:"legend,omitempty"`
	Scheme      string     `json:"scheme,omitempty"`
	Tiles       []string   `json:"tiles"`
	Grids       []string   `json:"grids,omitempty"`
	Data        []string   `json:"data,omitempty"`
	MinZoom     int        `json:"minzoom,omitempty"`
	MaxZoom     int        `json:"maxzoom,omitempty"`
	Bounds      [4]float64 `json:"bounds,omitempty"`
	Center      [3]float64 `json:"center,omitempty"`

	// Custom fields
	Format string `json:"format,omitempty"`
	Type   string `json:"type,omitempty"`

	*mbtiles.LayerData `json:",omitempty"`
}

// GetTileJSON returns a TileJSON by given tileset ID
func GetTileJSON(id string, u *url.URL) (*TileJSON, error) {
	ts, err := mbtiles.GetTileset(id)
	if err != nil {
		switch err {
		case mbtiles.ErrTilesetNotFound:
			return nil, fmt.Errorf("%w: %v", ErrNoEntity, err)
		default:
			return nil, err
		}
	}

	tsURL := fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, u.EscapedPath())
	query := ""
	if q := u.Query().Encode(); len(q) > 0 {
		query = "?" + q
	}

	md, err := ts.GetMetadata()
	if err != nil {
		return nil, err
	}

	tj := &TileJSON{
		TileJSON:    tileJSONVersion,
		Name:        md.Name,
		Description: md.Description,
		Version:     md.Version,
		Attribution: md.Attribution,
		Scheme:      tileJSONScheme,
		Format:      md.Format.String(),
		Type:        md.Type.String(),
		Tiles: []string{
			fmt.Sprintf("%s/tiles/{z}/{x}/{y}.%s%s", tsURL, ts.Format, query),
		},
		MinZoom:   md.MinZoom,
		MaxZoom:   md.MaxZoom,
		Bounds:    md.Bounds,
		Center:    md.Center,
		LayerData: md.LayerData,
	}

	if ts.UTFGrid {
		tj.Grids = []string{fmt.Sprintf("%s/tiles/{z}/{x}/{y}.json%s", tsURL, query)}
	}

	return tj, nil
}

type Tile struct {
	Data     []byte
	Format   mbtiles.TileFormat
	Modified time.Time
	Hash     [32]byte
}

func GetTile(id, z, x, y string) (*Tile, error) {
	ts, err := mbtiles.GetTileset(id)
	if err != nil {
		return nil, err
	}

	tc, err := mbtiles.ParseTileCoord(z, x, y)
	if err != nil {
		return nil, err
	}

	data, err := ts.GetTile(tc)
	if err != nil {
		return nil, err
	}

	h := blake3.New()
	h.Write(data)

	sum := h.Sum(nil)

	tile := &Tile{
		Data:     data,
		Format:   ts.Format,
		Modified: ts.Timestamp,
		Hash:     [32]byte(sum),
	}

	return tile, nil
}

func GetGrid(id, z, x, y string) (*Tile, error) {
	ts, err := mbtiles.GetTileset(id)
	if err != nil {
		return nil, err
	}

	tc, err := mbtiles.ParseTileCoord(z, x, y)
	if err != nil {
		return nil, err
	}

	data, err := ts.GetGrid(tc)
	if err != nil {
		return nil, err
	}

	tile := &Tile{
		Data:   data,
		Format: ts.UTFGridCompression,
	}

	return tile, nil
}
