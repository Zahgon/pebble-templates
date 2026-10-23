# Pebble

Pebble is a templating engine inspired by [Twig](https://twig.symfony.com/). It separates itself from the crowd with
its inheritance feature and its easy-to-read syntax. It ships with built-in autoescaping for security.

This repository holds the Go port of the Pebble core: the lexer, the parser and the evaluation of the whole template
language, with the tags, filters, functions, tests and operators of the core extension, the escaper extension with
its `html`, `js`, `css` and `url_param` strategies, template inheritance, `include`, `embed`, macros and imports, the
file loader and the `strictVariables` and `newLineTrimming` options. The port reproduces the behaviour of the Java
release 4.1.2 byte for byte, down to the way the Java runtime formats numbers, maps case, formats dates, orders hash
maps and words its exceptions.

For documentation of the template language please visit the [website](https://pebbletemplates.io), or read `docs/`
in this repository.

## Building

Go 1.23 or newer, standard library only:

    CGO_ENABLED=0 go build -o pebble ./cmd/pebble

## The pebble command

    pebble [options] TEMPLATE_DIR TEMPLATE_NAME [CONTEXT_FILE]

It renders `TEMPLATE_NAME`, loaded from `TEMPLATE_DIR`, with the JSON object of `CONTEXT_FILE` as its context, and
writes the result to standard output. A failure prints the exception class, its message with the template name and
line, and each cause, on standard error, and exits 2 for a parser error, 3 for a loader error, 4 for any other
Pebble error, 64 for a usage error, 65 for a bad context, 66 for an unreadable context file, 70 for any other
failure and 74 for an output failure.

| option | meaning |
| :-- | :-- |
| `--strict-variables` | missing variables and attributes raise an error instead of rendering nothing |
| `--no-newline-trimming` | keep the newline that follows a tag |
| `--no-autoescaping` | render expressions unescaped |
| `--escaping-strategy=NAME` | the default escaping strategy: `html` (the default), `js`, `css` or `url_param` |
| `--locale=TAG` | the locale of the evaluation, as a language tag; `en-US` by default |
| `--timezone=ZONE` | the default time zone; `UTC` by default |
| `--literal-decimals-as-integers` | number literals with a fraction become integers |
| `--literal-numbers-as-bigdecimals` | number literals become big decimals |
| `--max-rendered-size=N` | fail once the output passes N characters (`-1`, the default, is unlimited) |
| `--big-decimals` | context numbers with a fraction or an exponent become big decimals |
| `--hash-maps` | context objects become hash maps, so their iteration order is the hash order |
| `--var=NAME=JSON` | add one context entry, repeatable |

## Using the library

    engine := pebble.NewEngine(&pebble.Options{Prefix: "/srv/templates", NewLineTrimming: true,
        AutoEscaping: true, DefaultEscapingStrategy: "html", MaxRenderedSize: -1,
        Locale: "en-US", LocaleDisplay: "en_US", TimeZone: "UTC"})
    context := &pebble.Map{Vals: map[any]any{}}
    err := engine.Render(os.Stdout, "index.peb", context)

## Tests

    go test ./...

The tests cover the lexer and the parser, the Java number, string and date semantics the output depends on, the
filters, functions, tests and operators, and the command, replaying recorded command lines of the Java release.
