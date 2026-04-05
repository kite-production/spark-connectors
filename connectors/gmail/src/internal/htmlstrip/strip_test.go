package htmlstrip

import "testing"

func TestStrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain text passthrough",
			input: "Hello world",
			want:  "Hello world",
		},
		{
			name:  "simple tags removed",
			input: "<b>bold</b> and <i>italic</i>",
			want:  "bold and italic",
		},
		{
			name:  "br to newline",
			input: "line1<br>line2<br/>line3",
			want:  "line1\nline2\nline3",
		},
		{
			name:  "paragraph tags",
			input: "<p>paragraph one</p><p>paragraph two</p>",
			want:  "paragraph one\n\nparagraph two",
		},
		{
			name:  "script blocks removed",
			input: "before<script>alert('xss')</script>after",
			want:  "beforeafter",
		},
		{
			name:  "style blocks removed",
			input: "text<style>.foo{color:red}</style>more",
			want:  "textmore",
		},
		{
			name:  "html entities decoded",
			input: "A &amp; B &lt; C &gt; D &quot;E&quot; F&#39;s",
			want:  `A & B < C > D "E" F's`,
		},
		{
			name:  "nbsp decoded",
			input: "hello&nbsp;world",
			want:  "hello world",
		},
		{
			name:  "whitespace collapsed",
			input: "  lots   of    spaces  ",
			want:  "lots of spaces",
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
		{
			name:  "nested tags",
			input: "<div><p><span>nested</span></p></div>",
			want:  "nested",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Strip(tt.input)
			if got != tt.want {
				t.Errorf("Strip() = %q, want %q", got, tt.want)
			}
		})
	}
}
