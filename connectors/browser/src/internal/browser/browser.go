// Package browser provides a Chrome DevTools Protocol wrapper using chromedp.
package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"

	"github.com/chromedp/chromedp"
)

// Browser wraps chromedp to provide a high-level browser automation API.
type Browser struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc

	mu      sync.Mutex
	tabCtx  context.Context
	tabCanc context.CancelFunc
}

// New creates a new Browser instance with a chromedp allocator.
// If chromePath is empty, chromedp will auto-detect the Chrome installation.
func New(headless bool, chromePath string) (*Browser, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	if chromePath != "" {
		opts = append(opts, chromedp.ExecPath(chromePath))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	// Create the initial browser tab context.
	tabCtx, tabCanc := chromedp.NewContext(allocCtx)

	// Navigate to about:blank to start the browser process.
	if err := chromedp.Run(tabCtx); err != nil {
		allocCancel()
		return nil, fmt.Errorf("starting browser: %w", err)
	}

	return &Browser{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		tabCtx:      tabCtx,
		tabCanc:     tabCanc,
	}, nil
}

// Navigate navigates the browser to the given URL.
func (b *Browser) Navigate(ctx context.Context, url string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return chromedp.Run(b.tabCtx, chromedp.Navigate(url))
}

// Screenshot takes a full-page screenshot and returns it as base64-encoded PNG.
func (b *Browser) Screenshot(ctx context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var buf []byte
	if err := chromedp.Run(b.tabCtx, chromedp.FullScreenshot(&buf, 90)); err != nil {
		return "", fmt.Errorf("taking screenshot: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// Click clicks on the element matching the given CSS selector.
func (b *Browser) Click(ctx context.Context, selector string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return chromedp.Run(b.tabCtx, chromedp.Click(selector, chromedp.ByQuery))
}

// Type types text into the element matching the given CSS selector.
func (b *Browser) Type(ctx context.Context, selector, text string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return chromedp.Run(b.tabCtx,
		chromedp.Click(selector, chromedp.ByQuery),
		chromedp.SendKeys(selector, text, chromedp.ByQuery),
	)
}

// Evaluate executes JavaScript in the browser and returns the result as a string.
func (b *Browser) Evaluate(ctx context.Context, script string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var result interface{}
	if err := chromedp.Run(b.tabCtx, chromedp.Evaluate(script, &result)); err != nil {
		return "", fmt.Errorf("evaluating script: %w", err)
	}
	return fmt.Sprintf("%v", result), nil
}

// GetText extracts the text content of the element matching the given CSS selector.
func (b *Browser) GetText(ctx context.Context, selector string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var text string
	if err := chromedp.Run(b.tabCtx, chromedp.Text(selector, &text, chromedp.ByQuery)); err != nil {
		return "", fmt.Errorf("getting text: %w", err)
	}
	return text, nil
}

// Alive checks whether the browser process is still responsive.
func (b *Browser) Alive() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.tabCtx == nil {
		return false
	}
	// Try a simple evaluation to check responsiveness.
	var result interface{}
	err := chromedp.Run(b.tabCtx, chromedp.Evaluate("1+1", &result))
	return err == nil
}

// Close shuts down the browser and releases all resources.
func (b *Browser) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.tabCanc != nil {
		b.tabCanc()
	}
	if b.allocCancel != nil {
		b.allocCancel()
	}
}
