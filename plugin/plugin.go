package plugin

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/crane-editor/crane/log"

	"github.com/sourcegraph/jsonrpc2"
)

//
const (
	EditPriorityHigh = 0x10000000
)

// Plugin is
type Plugin struct {
	Views            map[string]*View
	conn             *jsonrpc2.Conn
	Stop             chan struct{}
	id               int
	handleFunc       HandleFunc
	handleBeforeFunc HandleFunc
	Mutex            sync.Mutex
}

// Config is
type Config struct {
	AutoIndent            bool          `json:"auto_indent"`
	FontFace              string        `json:"font_face"`
	FontSize              float64       `json:"font_size"`
	LineEnding            string        `json:"line_ending"`
	PluginSearchPath      []interface{} `json:"plugin_search_path"`
	ScrollPastEnd         bool          `json:"scroll_past_end"`
	TabSize               int           `json:"tab_size"`
	TranslateTabsToSpaces bool          `json:"translate_tabs_to_spaces"`
	UseTabStops           bool          `json:"use_tab_stops"`
	WrapWidth             int           `json:"wrap_width"`
}

// BufferInfo is
type BufferInfo struct {
	BufSize  int      `json:"buf_size"`
	BufferID int      `json:"buffer_id"`
	Config   *Config  `json:"config"`
	NbLines  int      `json:"nb_lines"`
	Path     string   `json:"path"`
	Rev      uint64   `json:"rev"`
	Syntax   string   `json:"syntax"`
	Views    []string `json:"views"`
}

// Initialization is
type Initialization struct {
	BufferInfo []*BufferInfo `json:"buffer_info"`
	PluginID   int           `json:"plugin_id"`
}

// Edit is
type Edit struct {
	Rev         uint64 `json:"rev"`
	Delta       *Delta `json:"delta"`
	Priority    uint64 `json:"priority"`
	AfterCursor bool   `json:"after_cursor"`
	Author      string `json:"author"`
	NewLen      int    `json:"new_len,omitempty"`
}

// HandleFunc is
type HandleFunc func(req interface{}) (interface{}, bool)

// NewPlugin is
func NewPlugin() *Plugin {
	p := &Plugin{
		Stop:  make(chan struct{}),
		Views: map[string]*View{},
	}
	p.conn = jsonrpc2.NewConn(context.Background(), NewStdinoutStream(), p)
	return p
}

// SetHandleFunc is
func (p *Plugin) SetHandleFunc(handleFunc HandleFunc) {
	p.handleFunc = handleFunc
}

// SetHandleBeforeFunc is
func (p *Plugin) SetHandleBeforeFunc(handleBeforeFunc HandleFunc) {
	p.handleBeforeFunc = handleBeforeFunc
}

// Handle incoming
func (p *Plugin) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	defer func() {
		if r := recover(); r != nil {
			log.Infoln("handle error", r, string(debug.Stack()))
		}
	}()
	p.Mutex.Lock()
	defer p.Mutex.Unlock()

	params, err := req.Params.MarshalJSON()
	if err != nil {
		log.Infoln(err)
		return
	}
	log.Infoln("now handle", req.ID, req.Method, string(params))
	switch req.Method {
	case "initialize":
		var initialization *Initialization
		err := json.Unmarshal(params, &initialization)
		if err != nil {
			log.Infoln("initialize error")
			log.Infoln(err)
			return
		}
		p.id = initialization.PluginID
		for _, buf := range initialization.BufferInfo {
			p.initBuf(buf)
		}
		if p.handleFunc != nil {
			p.handleFunc(initialization)
		}
	case "new_buffer":
		var initialization *Initialization
		err := json.Unmarshal(params, &initialization)
		if err != nil {
			log.Infoln(err)
			return
		}
		for _, buf := range initialization.BufferInfo {
			p.initBuf(buf)
		}
		if p.handleFunc != nil {
			p.handleFunc(initialization)
		}
	case "update":
		var result interface{}
		defer func() {
			if result == nil {
				result = 0
			}
			p.conn.Reply(context.Background(), req.ID, result)
		}()

		var update *Update
		err := json.Unmarshal(params, &update)
		if err != nil {
			log.Infoln(err)
			return
		}

		if p.handleBeforeFunc != nil {
			var overide bool
			result, overide = p.handleBeforeFunc(update)
			if overide {
				return
			}
		}

		p.Views[update.ViewID].Cache.ApplyUpdate(update)

		if p.handleFunc != nil {
			result, _ = p.handleFunc(update)
		}
	}
	log.Infoln("handle done")
}

func (p *Plugin) initBuf(buf *BufferInfo) {
	// lineCache := &LineCache{
	// 	ViewID: buf.Views[0],
	// }
	syntax := filepath.Ext(buf.Path)
	if strings.HasPrefix(syntax, ".") {
		syntax = string(syntax[1:])
	}
	for _, viewID := range buf.Views {
		p.Views[viewID] = &View{
			ID:     viewID,
			Path:   buf.Path,
			Syntax: syntax,
			Cache:  &Cache{},
			// LineCache: lineCache,
		}
	}
}

// Edit is
func (p *Plugin) Edit(view *View, edit *Edit) {
	params := map[string]interface{}{}
	params["edit"] = edit
	// params["msg"] = 0
	params["view_id"] = view.ID
	params["plugin_id"] = p.id
	p.conn.Notify(context.Background(), "edit", params)
}