// Package mbtiles contains some code parts borrowed from github.com/consbio/mbtileserver which is released under ISC.
package mbtiles

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/logger"
	_ "github.com/mattn/go-sqlite3" // import sqlite3 driver
)

var (
	ErrInvalidTileFormat        = errors.New("invalid or unknown tile format")
	ErrUnknownTileFormatPattern = errors.New("unknown tile format pattern")
	ErrInvalidLayerType         = errors.New("invalid or unknown layer type")
	ErrTilesetNotFound          = errors.New("tileset not found")
	ErrTileNotFound             = errors.New("tile not found")
	ErrNoUTFGrid                = errors.New("tileset does not contain UTF grids")
	ErrInvalidTileCoord         = errors.New("tile coordinates are not valid")
)

const fileExtension = ".mbtiles"

var tilesets = map[string]*Tileset{}

// LoadTilesets creates a Tileset of all MBTiles in the specified directory
// and adds them to the internal map
func LoadTilesets(path string) error {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return fmt.Errorf("reading tileset directory failed: %w", err)
	}

	ch := make(chan *Tileset, 1)
	wg := &sync.WaitGroup{}

	for _, f := range files {
		if !f.IsDir() && filepath.Ext(f.Name()) == fileExtension {
			wg.Add(1)
			go func() {
				ts, err := NewTileset(fmt.Sprintf("%s/%s", path, f.Name()))
				if err != nil {
					logger.Errorf("Loading tileset \"%s\" failed: %s", f.Name(), err)
				}
				ch <- ts
				wg.Done()
			}()
		}
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for ts := range ch {
		tilesets[strings.TrimSuffix(ts.Filename, fileExtension)] = ts
	}

	logger.Infof("%v tileset(s) loaded successfully", len(tilesets))

	return nil
}

// GetTileset returns a Tileset by the given ID
func GetTileset(id string) (*Tileset, error) {
	if ts, ok := tilesets[id]; ok {
		return ts, nil
	}

	return nil, ErrTilesetNotFound
}

// TileFormat represents the format of a tile
type TileFormat int

const (
	UNKNOWN TileFormat = iota
	PBF
	PNG
	JPG
	WEBP
	GZIP
	ZLIB
)

var formatStrings = [...]string{
	"",
	"pbf",
	"png",
	"jpg",
	"webp",
	"gzib",
	"zlib",
}

// String returns a string representing the TileFormat
func (f TileFormat) String() string {
	return formatStrings[f]
}

func (f *TileFormat) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.String())
}

func (f *TileFormat) UnmarshalJSON(b []byte) error {
	var format string

	if err := json.Unmarshal(b, &format); err != nil {
		return err
	}

	for i, k := range formatStrings {
		if k == format {
			*f = TileFormat(i)
			return nil
		}
	}

	return ErrInvalidTileFormat
}

// ContentType returns the MIME content type of the TileFormat
func (f TileFormat) ContentType() string {
	switch f {
	case PNG:
		return "image/png"
	case JPG:
		return "image/jpeg"
	case PBF:
		return "application/x-protobuf" // Content-Encoding header must be gzip
	case WEBP:
		return "image/webp"
	default:
		return ""
	}
}

func stringToTileFormat(s string) TileFormat {
	for i, k := range formatStrings {
		if k == s {
			return TileFormat(i)
		}
	}

	return UNKNOWN
}

// LayerType represents the MBTiles layer type
type LayerType int

const (
	BaseLayer LayerType = iota
	Overlay
)

var layerTypeStrings = [...]string{
	"baselayer",
	"overlay",
}

// String returns a string representing the LayerType
func (t LayerType) String() string {
	return layerTypeStrings[t]
}

func (t *LayerType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t *LayerType) UnmarshalJSON(b []byte) error {
	var lt string

	if err := json.Unmarshal(b, &lt); err != nil {
		return err
	}

	for i, k := range layerTypeStrings {
		if k == lt {
			*t = LayerType(i)
			return nil
		}
	}

	return ErrInvalidTileFormat
}

func stringToLayerType(s string) LayerType {
	for i, k := range layerTypeStrings {
		if k == s {
			return LayerType(i)
		}
	}

	return BaseLayer
}

// Tileset represents an MBTiles instance
type Tileset struct {
	Filename           string
	Format             TileFormat
	Timestamp          time.Time
	UTFGrid            bool
	UTFGridCompression TileFormat

	database *sql.DB
}

// NewTileset creates a new Tileset by the given MBTiles file
func NewTileset(file string) (*Tileset, error) {
	fileStat, err := os.Stat(file)
	if err != nil {
		return nil, fmt.Errorf("could not read file stats for mbtiles file: %w", err)
	}

	db, err := sql.Open("sqlite3", file)
	if err != nil {
		return nil, err
	}

	// Validate the mbtiles file
	// 'tiles', 'metadata' tables or views must be present
	var tableCount int
	if err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name IN ('tiles', 'metadata')").Scan(&tableCount); err != nil {
		return nil, err
	}

	if tableCount < 2 {
		return nil, fmt.Errorf("missing required table: 'tiles' OR 'metadata'")
	}

	// Query a sample tile to determine format
	var data []byte
	if err = db.QueryRow("SELECT tile_data FROM tiles LIMIT 1").Scan(&data); err != nil {
		return nil, err
	}

	format, err := detectTileFormat(data)
	if err != nil {
		return nil, err
	}

	if format == GZIP {
		format = PBF // GZIP masks PBF, which is only expected type for tiles in GZIP format
	}

	if format != PBF {
		return nil, fmt.Errorf("The tile format \"%s\" is currently not supported", format)
	}

	ts := &Tileset{
		Filename:  fileStat.Name(),
		Format:    format,
		Timestamp: fileStat.ModTime().Round(time.Second),
		database:  db,
	}

	// UTFGrids
	// first check to see if requisite views exist: grids, grid_data
	// by convention, these views are queries against: grid_utfgrid, keymap, grid_key
	// for some mbtiles files, all tables and views will be present, but not have any UTFGrids
	// since the grids view is a join against 'map' table on 'grid_id', querying this view for any grids
	// may take a very long time for large tilesets if there are not actually any grids.
	// For this reason, we query data directly from 'grid_utfgrid' to determine if any grids are present.
	// NOTE: this assumption may not be valid for all mbtiles files, since grid_utfgrid is used by convention
	// rather than specification.
	var count int
	if err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name IN ('grids', 'grid_data', 'grid_utfgrid', 'keymap', 'grid_key')").
		Scan(&count); err != nil {
		return nil, err
	}

	if count == 5 {
		// query a sample grid to detect type
		var gridData []byte
		if err = db.QueryRow("SELECT grid_utfgrid FROM grid_utfgrid LIMIT 1").Scan(&gridData); err != nil {
			if err != sql.ErrNoRows {
				return nil, fmt.Errorf("could not read sample grid to determine type: %w", err)
			}
		} else {
			ts.UTFGrid = true
			ts.UTFGridCompression, err = detectTileFormat(gridData)
			if err != nil {
				return nil, fmt.Errorf("could not determine UTF Grid compression type: %w", err)
			}
		}
	}

	return ts, nil
}

type TileCoord struct {
	Z    uint8
	X, Y uint64
}

// ParseTileCoord parses and returns TileCoord coordinates and an optional
// extension from the three parameters. The parameter z is interpreted as the
// web mercator zoom level, it is supposed to be an unsigned integer that will
// fit into 8 bit. The parameters x and y are interpreted as longitude and
// latitude tile indices for that zoom level, both are supposed be integers in
// the integer interval [0,2^z). Additionally, y may also have an optional
// filename extension (e.g. "42.png") which is removed before parsing the
// number, and returned, too. In case an error occured during parsing or if the
// values are not in the expected interval, the returned error is non-nil.
func ParseTileCoord(z, x, y string) (tc *TileCoord, err error) {
	var z64 uint64
	if z64, err = strconv.ParseUint(z, 10, 8); err != nil {
		err = fmt.Errorf("%w: cannot parse zoom level: %s", ErrInvalidTileCoord, err)
		return
	}

	tc = &TileCoord{}

	tc.Z = uint8(z64)

	const errTplParse = "cannot parse %s coordinate axis: %w"
	const errTplOOB = "%s coordinate (%d) is out of bounds for zoom level %d"

	if tc.X, err = strconv.ParseUint(x, 10, 64); err != nil {
		err = fmt.Errorf("%w: %v", ErrInvalidTileCoord, fmt.Errorf(errTplParse, "first", err))
		return
	}

	if tc.X >= (1 << z64) {
		err = fmt.Errorf("%w: %v", ErrInvalidTileCoord, fmt.Errorf(errTplOOB, "x", tc.X, tc.Z))
		return
	}

	s := y
	if l := strings.LastIndex(s, "."); l >= 0 {
		s = s[:l]
	}

	if tc.Y, err = strconv.ParseUint(s, 10, 64); err != nil {
		err = fmt.Errorf("%w: %v", ErrInvalidTileCoord, fmt.Errorf(errTplParse, "y", err))
		return
	}

	if tc.Y >= (1 << z64) {
		err = fmt.Errorf("%w: %v", ErrInvalidTileCoord, fmt.Errorf(errTplOOB, "y", tc.Y, tc.Z))
		return
	}

	tc.Y = (1 << uint64(tc.Z)) - 1 - tc.Y

	return
}

// GetTile reads a tile with tile identifiers z, x, y into []byte.
func (ts *Tileset) GetTile(tc *TileCoord) ([]byte, error) {
	var data []byte

	if err := ts.database.QueryRow("SELECT tile_data FROM tiles WHERE zoom_level = ? AND tile_column = ? AND tile_row = ?", tc.Z, tc.X, tc.Y).
		Scan(&data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return data, ErrTileNotFound
		}
		return data, err
	}

	return data, nil
}

// GetGrid reads a UTFGrid with identifiers z, x, y into []byte.
// This merges in grid key data. The data is returned in the original compression encoding (zlib or gzip)
func (ts *Tileset) GetGrid(tc *TileCoord) ([]byte, error) {
	var data []byte

	if !ts.UTFGrid {
		return data, ErrNoUTFGrid
	}

	if err := ts.database.QueryRow("SELECT grid FROM grids WHERE zoom_level = ? AND tile_column = ? AND tile_row = ?", tc.Z, tc.X, tc.Y).
		Scan(&data); err != nil {
		return data, err
	}

	rows, err := ts.database.Query("SELECT key_name, key_json FROM grid_data WHERE zoom_level = ? AND tile_column = ? AND tile_row = ?", tc.Z, tc.X, tc.Y)
	if err != nil {
		return data, fmt.Errorf("cannot fetch grid data: %w", err)
	}
	defer rows.Close()

	keydata := make(map[string]interface{})
	var key string
	var value []byte

	for rows.Next() {
		if err := rows.Scan(&key, &value); err != nil {
			return data, fmt.Errorf("could not fetch grid data: %w", err)
		}

		valuejson := make(map[string]interface{})
		json.Unmarshal(value, &valuejson)
		keydata[key] = valuejson
	}

	if len(keydata) == 0 {
		return data, nil // there is no key data for this tile, return
	}

	var zreader io.ReadCloser  // instance of zlib or gzip reader
	var zwriter io.WriteCloser // instance of zlip or gzip writer
	var buf bytes.Buffer

	reader := bytes.NewReader(data)

	switch ts.UTFGridCompression {
	case ZLIB:
		zreader, err = zlib.NewReader(reader)
		zwriter = zlib.NewWriter(&buf)
	case GZIP:
		zreader, err = gzip.NewReader(reader)
		zwriter = gzip.NewWriter(&buf)
	default:
		err = fmt.Errorf("unknown grid compression")
	}
	if err != nil {
		return data, err
	}

	var utfjson map[string]interface{}
	if err = json.NewDecoder(zreader).Decode(&utfjson); err != nil {
		return data, err
	}
	zreader.Close()

	// splice the key data into the UTF json
	utfjson["data"] = keydata

	// now re-encode to original zip encoding
	if err = json.NewEncoder(zwriter).Encode(utfjson); err != nil {
		return data, err
	}
	zwriter.Close()

	buf.Write(data)

	return data, nil
}

// GetMetadata reads the metadata table into Metadata, casting their values into
// the appropriate type
func (ts *Tileset) GetMetadata() (*Metadata, error) {
	md := &Metadata{}

	rows, err := ts.database.Query("SELECT * FROM metadata WHERE value is not ''")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var key, value string
	for rows.Next() {
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}

		switch key {
		case "name":
			md.Name = value
		case "description":
			md.Description = value
		case "attribution":
			md.Attribution = value
		case "version":
			md.Version = value
		case "format":
			md.Format = stringToTileFormat(value)
		case "minzoom":
			md.MinZoom, err = strconv.Atoi(value)
		case "maxzoom":
			md.MaxZoom, err = strconv.Atoi(value)
		case "center":
			md.Center, err = stringToCenter(value)
		case "bounds":
			md.Bounds, err = stringToBounds(value)
		case "type":
			md.Type = stringToLayerType(value)
		case "json":
			err = json.Unmarshal([]byte(value), &md.LayerData)
		}

		if err != nil {
			return nil, err
		}
	}

	if md.MaxZoom == 0 {
		var min, max string
		if err := ts.database.QueryRow("SELECT min(zoom_level), max(zoom_level) FROM tiles").Scan(&min, &max); err != nil {
			return nil, err
		}

		md.MinZoom, err = strconv.Atoi(min)
		if err != nil {
			return nil, err
		}

		md.MaxZoom, err = strconv.Atoi(max)
		if err != nil {
			return nil, err
		}
	}

	return md, nil
}

// ContentType returns the content-type string of the TileFormat of the Tileset.
func (ts *Tileset) ContentType() string {
	return ts.Format.ContentType()
}

// Close closes the database connection of the Tileset
func (ts *Tileset) Close() error {
	return ts.database.Close()
}

type Metadata struct {
	Name        string     `json:"name"`
	Format      TileFormat `json:"format"`
	Bounds      [4]float64 `json:"bounds,omitempty"`
	Center      [3]float64 `json:"center,omitempty"`
	MinZoom     int        `json:"minzoom,omitempty"`
	MaxZoom     int        `json:"maxzoom,omitempty"`
	Description string     `json:"description,omitempty"`
	Version     string     `json:"version,omitempty"`
	Type        LayerType  `json:"type,omitempty"`
	Attribution string     `json:"attribution,omitempty"`
	LayerData   *LayerData `json:"layerData,omitempty"`
}

type LayerData struct {
	VectorLayers *[]VectorLayer `json:"vector_layers,omitempty"`
	TileStats    *TileStats     `json:"tilestats,omitempty"`
}

type VectorLayer struct {
	ID          string                 `json:"id"`
	Fields      map[string]interface{} `json:"fields"`
	Description string                 `json:"description,omitempty"`
	MinZoom     int                    `json:"minzoom,omitempty"`
	MaxZoom     int                    `json:"maxzoom,omitempty"`
}

type TileStats struct {
	LayerCount int     `json:"layerCount"`
	Layers     []Layer `json:"layers"`
}

type Layer struct {
	Name           string      `json:"layer"`
	Count          int64       `json:"count"`
	Geometry       string      `json:"geometry"`
	AttributeCount int         `json:"attributeCount"`
	Attributes     []Attribute `json:"attributes,omitempty"`
}

type Attribute struct {
	Name   string        `json:"attribute"`
	Count  int           `json:"count"`
	Type   string        `json:"type"`
	Values []interface{} `json:"values"`
}

var tileFomatPatterns = map[TileFormat][]byte{
	GZIP: []byte("\x1f\x8b"), // this masks PBF format too
	ZLIB: []byte("\x78\x9c"),
	PNG:  []byte("\x89\x50\x4E\x47\x0D\x0A\x1A\x0A"),
	JPG:  []byte("\xFF\xD8\xFF"),
	WEBP: []byte("\x52\x49\x46\x46\xc0\x00\x00\x00\x57\x45\x42\x50\x56\x50"),
}

// detectFileFormat inspects the first few bytes of byte array to determine tile
// format PBF tile format does not have a distinct signature, it will be
// returned as GZIP, and it is up to caller to determine that it is a PBF format
func detectTileFormat(data []byte) (TileFormat, error) {
	for format, pattern := range tileFomatPatterns {
		if bytes.HasPrefix(data, pattern) {
			return format, nil
		}
	}

	return UNKNOWN, ErrUnknownTileFormatPattern
}

func stringToBounds(str string) (bounds [4]float64, err error) {
	for i, v := range strings.Split(str, ",") {
		bounds[i], err = strconv.ParseFloat(strings.TrimSpace(v), 64)
	}

	return
}

func stringToCenter(str string) (center [3]float64, err error) {
	for i, v := range strings.Split(str, ",") {
		center[i], err = strconv.ParseFloat(strings.TrimSpace(v), 64)
	}

	return
}
