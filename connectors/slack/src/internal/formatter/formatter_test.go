package formatter

import "testing"

func TestMarkdownToMrkdwn(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bold double asterisk",
			in:   "This is **bold** text",
			want: "This is *bold* text",
		},
		{
			name: "bold double underscore",
			in:   "This is __bold__ text",
			want: "This is *bold* text",
		},
		{
			name: "strikethrough",
			in:   "This is ~~deleted~~ text",
			want: "This is ~deleted~ text",
		},
		{
			name: "link",
			in:   "Click [here](https://example.com) now",
			want: "Click <https://example.com|here> now",
		},
		{
			name: "inline code preserved",
			in:   "Use `fmt.Println` function",
			want: "Use `fmt.Println` function",
		},
		{
			name: "code block preserved",
			in:   "```\nfunc main() {}\n```",
			want: "```\nfunc main() {}\n```",
		},
		{
			name: "header",
			in:   "# Title",
			want: "*Title*",
		},
		{
			name: "h2 header",
			in:   "## Subtitle",
			want: "*Subtitle*",
		},
		{
			name: "combined formatting",
			in:   "**bold** and ~~strike~~ and [link](http://x.com)",
			want: "*bold* and ~strike~ and <http://x.com|link>",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "plain text unchanged",
			in:   "Hello world",
			want: "Hello world",
		},
		{
			name: "multiple links",
			in:   "[a](http://a.com) and [b](http://b.com)",
			want: "<http://a.com|a> and <http://b.com|b>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MarkdownToMrkdwn(tt.in)
			if got != tt.want {
				t.Errorf("MarkdownToMrkdwn(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
