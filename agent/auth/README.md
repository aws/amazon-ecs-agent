## How to test secrets manager

1) modify constants file to include the region and profile name (--profile) you want to test with

2) run `go test -tags setup`

3) run `go test -tags integ`

4) modify the auth code to understand how it works

5) profit
