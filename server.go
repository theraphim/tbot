package tbot

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	apiBaseURL = "https://api.telegram.org"
)

// Server will connect and serve all updates from Telegram
type Server struct {
	ctx    context.Context
	cancel func()

	listeningSocket net.Listener

	webhookURL string
	listenAddr string
	baseURL    string
	httpClient *http.Client
	client     *Client
	token      string
	logger     Logger
	bufferSize int
	nextOffset int

	messageHandlers        map[string]handlerFunc
	defaultMessageHandler  handlerFunc
	editMessageHandler     handlerFunc
	channelPostHandler     handlerFunc
	editChannelPostHandler handlerFunc
	inlineQueryHandler     func(*InlineQuery)
	inlineResultHandler    func(*ChosenInlineResult)
	callbackHandler        func(*CallbackQuery)
	shippingHandler        func(*ShippingQuery)
	preCheckoutHandler     func(*PreCheckoutQuery)
	pollHandler            func(*Poll)
	pollAnswerHandler      func(*PollAnswer)

	//	middlewares []Middleware
}

// UpdateHandler is a function for middlewares
type UpdateHandler func(*Update)

// Middleware is a middleware for updates
type Middleware func(UpdateHandler) UpdateHandler

// ServerOption type for additional Server options
type ServerOption func(*Server)

type handlerFunc func(*Message)

/*
New creates new Server. Available options:
	WithWebhook(url, addr string)
	WithHTTPClient(client *http.Client)
	WithBaseURL(baseURL string)
*/
func New(token string, options ...ServerOption) *Server {
	s := &Server{
		httpClient: http.DefaultClient,
		token:      token,
		logger:     nopLogger{},
		baseURL:    apiBaseURL,
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())

	for _, opt := range options {
		opt(s)
	}
	// bot, err :=  tgbotapi.NewBotAPIWithClient(token, s.httpClient)
	s.client = NewClient(token, s.httpClient, s.baseURL)
	return s
}

// WithWebhook returns ServerOption for given Webhook URL and Server address to listen.
// e.g. WithWebhook("https://bot.example.com/super/url", "0.0.0.0:8080")
func WithWebhook(url, addr string) ServerOption {
	return func(s *Server) {
		s.webhookURL = url
		s.listenAddr = addr
	}
}

// WithBaseURL sets custom apiBaseURL for server.
// It may be necessary to run the server in some countries
func WithBaseURL(baseURL string) ServerOption {
	return func(s *Server) {
		s.baseURL = baseURL
	}
}

// WithHTTPClient sets custom http client for server.
func WithHTTPClient(client *http.Client) ServerOption {
	return func(s *Server) {
		s.httpClient = client
	}
}

// WithLogger sets logger for tbot
func WithLogger(logger Logger) ServerOption {
	return func(s *Server) {
		s.logger = logger
	}
}

// Use adds middleware to server
// func (s *Server) Use(m Middleware) {
// 	s.middlewares = append(s.middlewares, m)
// }

func (s *Server) processBatchOfUpdates(updates []*Update) {
	for _, v := range updates {
		go s.processSingleUpdate(v)
	}
}

func (s *Server) processSingleUpdate(update *Update) {
	switch {
	case update.Message != nil:
		s.handleMessage(update.Message)
	case update.EditedMessage != nil:
		if s.editChannelPostHandler != nil {
			s.editMessageHandler(update.EditedMessage)
		}
	case update.ChannelPost != nil:
		if s.channelPostHandler != nil {
			s.channelPostHandler(update.ChannelPost)
		}
	case update.EditedChannelPost != nil:
		if s.editChannelPostHandler != nil {
			s.editChannelPostHandler(update.EditedChannelPost)
		}
	case update.InlineQuery != nil:
		if s.inlineQueryHandler != nil {
			s.inlineQueryHandler(update.InlineQuery)
		}
	case update.ChosenInlineResult != nil:
		if s.inlineResultHandler != nil {
			s.inlineResultHandler(update.ChosenInlineResult)
		}
	case update.CallbackQuery != nil:
		if s.callbackHandler != nil {
			s.callbackHandler(update.CallbackQuery)
		}
	case update.ShippingQuery != nil:
		if s.shippingHandler != nil {
			s.shippingHandler(update.ShippingQuery)
		}
	case update.PreCheckoutQuery != nil:
		if s.preCheckoutHandler != nil {
			s.preCheckoutHandler(update.PreCheckoutQuery)
		}
	case update.Poll != nil:
		if s.pollHandler != nil {
			s.pollHandler(update.Poll)
		}
	case update.PollAnswer != nil:
		if s.pollAnswerHandler != nil {
			s.pollAnswerHandler(update.PollAnswer)
		}
	}
}

// Start listening for updates
func (s *Server) Start() error {
	if len(s.token) == 0 {
		return fmt.Errorf("token is empty")
	}
	if s.webhookURL != "" && s.listenAddr != "" {
		return s.listenUpdates()
	}
	// s.client.deleteWebhook()
	return s.processLongPollUpdates()
}

// Client returns Telegram API Client
func (s *Server) Client() *Client {
	return s.client
}

// Stop listening for updates
func (s *Server) Stop() {
	s.cancel()
	if s.listeningSocket != nil {
		s.listeningSocket.Close()
	}
}

func (s *Server) listenUpdates() error {
	err := s.client.setWebhook(s.webhookURL)
	if err != nil {
		return fmt.Errorf("unable to set webhook: %v", err)
	}
	handler := func(w http.ResponseWriter, r *http.Request) {
		up := &Update{}
		err := json.NewDecoder(r.Body).Decode(up)
		if err != nil {
			s.logger.Errorf("unable to decode update: %v", err)
			return
		}
		s.processSingleUpdate(up)
	}
	s.listeningSocket, err = net.Listen("tcp", s.listenAddr)
	if err != nil {
		return err
	}
	return http.Serve(s.listeningSocket, http.HandlerFunc(handler))
}

func (s *Server) processLongPollUpdates() error {
	s.logger.Debugf("fetching updates...")
	var endpoint strings.Builder
	endpoint.WriteString(s.baseURL)
	endpoint.WriteString("/bot")
	endpoint.WriteString(s.token)
	endpoint.WriteString("/getUpdates")
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	params := url.Values{}
	params.Set("timeout", "60")
	for {
		params.Set("offset", strconv.Itoa(s.nextOffset))
		ctx, cancel := context.WithTimeout(s.ctx, time.Second*120)
		dreq := req.WithContext(ctx)
		dreq.URL.RawQuery = params.Encode()
		resp, err := s.httpClient.Do(req)
		cancel()
		if err != nil {
			s.logger.Errorf("unable to perform request: %v", err)
			select {
			case <-time.After(time.Second * 5):
			case <-s.ctx.Done():
				return s.ctx.Err()
			}
			continue
		}
		var updatesResp *struct {
			OK          bool      `json:"ok"`
			Result      []*Update `json:"result"`
			Description string    `json:"description"`
		}
		err = json.NewDecoder(resp.Body).Decode(&updatesResp)
		if err != nil {
			s.logger.Errorf("unable to decode response: %v", err)
			resp.Body.Close()
			select {
			case <-time.After(time.Second * 5):
			case <-s.ctx.Done():
				return s.ctx.Err()
			}
			continue
		}
		err = resp.Body.Close()
		if err != nil {
			s.logger.Errorf("unable to close response body: %v", err)
		}
		if !updatesResp.OK {
			s.logger.Errorf("updates query fail: %s", updatesResp.Description)
			select {
			case <-time.After(time.Second * 5):
			case <-s.ctx.Done():
				return s.ctx.Err()
			}
			continue
		}
		if len(updatesResp.Result) == 0 {
			continue
		}
		s.nextOffset = updatesResp.Result[len(updatesResp.Result)-1].UpdateID + 1
		s.processBatchOfUpdates(updatesResp.Result)
	}
}

// HandleMessage sets handler for incoming messages
func (s *Server) HandleMessage(text string, handler func(*Message)) {
	if s.messageHandlers == nil {
		s.messageHandlers = make(map[string]handlerFunc)
	}
	s.messageHandlers[text] = handler
}

// HandleEditedMessage set handler for incoming edited messages
func (s *Server) HandleEditedMessage(handler func(*Message)) {
	s.editMessageHandler = handler
}

// HandleChannelPost set handler for incoming channel post
func (s *Server) HandleChannelPost(handler func(*Message)) {
	s.channelPostHandler = handler
}

// HandleEditChannelPost set handler for incoming edited channel post
func (s *Server) HandleEditChannelPost(handler func(*Message)) {
	s.editChannelPostHandler = handler
}

// HandleInlineQuery set handler for inline queries
func (s *Server) HandleInlineQuery(handler func(*InlineQuery)) {
	s.inlineQueryHandler = handler
}

// HandleInlineResult set inline result handler
func (s *Server) HandleInlineResult(handler func(*ChosenInlineResult)) {
	s.inlineResultHandler = handler
}

// HandleCallback set handler for inline buttons
func (s *Server) HandleCallback(handler func(*CallbackQuery)) {
	s.callbackHandler = handler
}

// HandleShipping set handler for shipping queries
func (s *Server) HandleShipping(handler func(*ShippingQuery)) {
	s.shippingHandler = handler
}

// HandlePreCheckout set handler for pre-checkout queries
func (s *Server) HandlePreCheckout(handler func(*PreCheckoutQuery)) {
	s.preCheckoutHandler = handler
}

// HandlePollUpdate set handler for anonymous poll updates
func (s *Server) HandlePollUpdate(handler func(*Poll)) {
	s.pollHandler = handler
}

// HandlePollAnswer set handler for non-anonymous poll updates
func (s *Server) HandlePollAnswer(handler func(*PollAnswer)) {
	s.pollAnswerHandler = handler
}

func (s *Server) handleMessage(msg *Message) {
	if h := s.messageHandlers[msg.Text]; h != nil {
		h(msg)
		return
	}
	if s.defaultMessageHandler != nil {
		s.defaultMessageHandler(msg)
	}
}

func (s *Server) HandleDefault(handler handlerFunc) {
	s.defaultMessageHandler = handler
}
