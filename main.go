package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/qProust/fo/ast"
	"github.com/qProust/fo/format"
	"github.com/qProust/fo/importer"
	"github.com/qProust/fo/parser"
	"github.com/qProust/fo/token"
	"github.com/qProust/fo/transform"
	"github.com/qProust/fo/types"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()

	app.Name = "Fo"
	app.Usage = "An experimental language which adds functional programming features to Go."
	app.Commands = []cli.Command{
		{
			Name:   "run",
			Usage:  "run a single .fo file",
			Action: run,
		},
		{
			Name:   "build",
			Usage:  "build a single .fo file",
			Action: build,
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}
}

func buildFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("could not open file: %s", err)
	}
	if !strings.HasSuffix(f.Name(), ".fo") {
		return "", fmt.Errorf("%s is not a Fo file (expected '.fo' extension)", f.Name())
	}

	// Parse file.
	fset := token.NewFileSet()
	nodes, err := parser.ParseFile(fset, f.Name(), f, 0)
	if err != nil {
		return "", err
	}

	// Check types.
	conf := types.Config{Importer: importer.Default()}
	info := &types.Info{
		Selections: map[*ast.SelectorExpr]*types.Selection{},
		Uses:       map[*ast.Ident]types.Object{},
	}
	pkg, err := conf.Check(f.Name(), fset, []*ast.File{nodes}, info)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	// Transform to pure Go and write the output.
	trans := &transform.Transformer{
		Fset: fset,
		Pkg:  pkg,
		Info: info,
	}
	transformed, err := trans.File(nodes)
	if err != nil {
		return "", err
	}
	outputName := strings.TrimSuffix(f.Name(), ".fo") + ".go"
	output, err := os.Create(outputName)
	if err != nil {
		return "", err
	}
	if err := format.Node(output, fset, transformed); err != nil {
		return "", err
	}
	return outputName, nil
}

func build(c *cli.Context) error {
	path := "."
	if c.Args().Present() {
		path = c.Args().First()
	}
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if !strings.HasSuffix(path, ".fo") {
			return nil
		}
		_, err = buildFile(path)
		if err != nil {
			return fmt.Errorf("error in '%s': %s", path, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk path: %s", err)
	}

	return nil
}


func run(c *cli.Context) error {
	// Read arguments and open file.
	if !c.Args().Present() || len(c.Args().Tail()) != 0 {
		return errors.New("run expects exactly one argument: the name of a Fo file to run")
	}
	outputName, err := buildFile(c.Args().First())
	if err != nil {
		return err
	}

	// Invoke Go command to run the resulting Go code.
	cmd := exec.Command("go", "run", outputName)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
