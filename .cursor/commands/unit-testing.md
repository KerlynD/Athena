# unit-testing

This command ensures that you should run the current unit tests

To run all tests in all packages within your current directory and its subdirectories, use the following command in your repository's root directory: 

```powershell
go test ./...
```

`./...` is a pattern that tells the go test command to look for and run all *_test.go files recursively through all subdirectories starting from the current location.

To see a detailed description of each test as it runs, add the -v flag (verbose):

```powershell
go test -v ./...
```