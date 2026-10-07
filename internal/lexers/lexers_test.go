package lexers

import (
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// tokens tokenises src and returns the type of each text where it first
// appears.
func tokens(t *testing.T, lang, src string) map[string]chroma.TokenType {
	t.Helper()
	lexer := lexers.Get(lang)
	if lexer == nil {
		t.Fatalf("no lexer for %s", lang)
	}
	it, err := lexer.Tokenise(nil, src)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]chroma.TokenType{}
	for _, tok := range it.Tokens() {
		if _, ok := got[tok.Value]; !ok {
			got[tok.Value] = tok.Type
		}
	}
	return got
}

func expect(t *testing.T, got map[string]chroma.TokenType, want map[string]chroma.TokenType) {
	t.Helper()
	for text, typ := range want {
		if got[text] != typ {
			t.Errorf("%q is %s, want %s", text, got[text], typ)
		}
	}
}

func TestNix(t *testing.T) {
	src := `{ pkgs, lib ? null, ... }:
let
  f = x: x + 1;
in {
  home.packages = with pkgs; [ git ];
  xdg.configFile."a".source = ./a;
  enable = cfg.enable;
  same = left == b;
}
`
	expect(t, tokens(t, "nix", src), map[string]chroma.TokenType{
		// Attribute paths being set
		"home":       chroma.NameTag,
		"packages":   chroma.NameTag,
		"configFile": chroma.NameTag,
		"source":     chroma.NameTag,
		"f":          chroma.NameTag,
		"enable":     chroma.NameTag,
		"same":       chroma.NameTag,
		// Function arguments
		"pkgs": chroma.NameVariable,
		"lib":  chroma.NameVariable,
		"x":    chroma.NameVariable,
		// Names read, not set
		"cfg":  chroma.Name,
		"git":  chroma.Name,
		"left": chroma.Name,
		// Unchanged from chroma
		"let":  chroma.Keyword,
		"null": chroma.NameConstant,
		"./a":  chroma.LiteralStringRegex,
	})
}

func TestConsole(t *testing.T) {
	src := "$ nix run .#switch\nbuilding...\n$ FOO=1 git status\n M README.md\n$\n"
	expect(t, tokens(t, "console", src), map[string]chroma.TokenType{
		"$":              chroma.GenericPrompt,
		"nix":            chroma.NameFunction,
		"git":            chroma.NameFunction,
		"FOO":            chroma.NameVariable,
		"building...\n":  chroma.GenericOutput,
		" M README.md\n": chroma.GenericOutput,
	})
}

func TestBash(t *testing.T) {
	src := `cd ~/bin
gh gist create --public -d "x" a.ts
deno test --allow-all x_test.ts | grep ok && echo done
FOO=1 git status
x=$(git rev-parse HEAD)
arr=(a b)
make CC=gcc all
makeWrapper $out/bin/x \
  --prefix PATH : y
if [ -n "$FOO" ]; then sudo rm -f z; fi
`
	expect(t, tokens(t, "bash", src), map[string]chroma.TokenType{
		// Commands where a command goes
		"gh":          chroma.NameFunction,
		"deno":        chroma.NameFunction,
		"grep":        chroma.NameFunction,
		"git":         chroma.NameFunction,
		"make":        chroma.NameFunction,
		"makeWrapper": chroma.NameFunction,
		"sudo":        chroma.NameFunction,
		"rm":          chroma.NameFunction,
		"cd":          chroma.NameBuiltin,
		"echo":        chroma.NameBuiltin,
		// Options
		"--public":    chroma.NameConstant,
		"-d":          chroma.NameConstant,
		"--allow-all": chroma.NameConstant,
		"--prefix":    chroma.NameConstant,
		"-n":          chroma.NameConstant,
		// Arguments, even when they look like a command or a builtin
		"test":      chroma.Text,
		"gist":      chroma.Text,
		"rev-parse": chroma.Text,
		"a":         chroma.Text,
		"all":       chroma.Text,
		"PATH":      chroma.Text,
		// Variables
		"FOO": chroma.NameVariable,
		"CC":  chroma.NameVariable,
	})
}
