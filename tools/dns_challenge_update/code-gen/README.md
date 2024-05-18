### DNS Challenge Update Data Structure Code Generator

This script is designed to automatically pull lego, update the `acmedns` module with new json file and create a function for automatically create a provider based on the given providers name and config json.

### Usage

To update the module, simply run update.sh 

```
./update.sh
```

The updated files will be written into the `acmedns`  folder. Then, you can copy it to the ACME module folder (or later-on a CICD pipeline will be made to do this automatically, but for now you have to manually copy it into the module under `src/mod/acme/`)

### Build for Windows 7 (NT6.1)

If you want to build Zoraxy with NT6.1, the lego version must be kept on or below 4.15.0. To build acmedns module for win7, add -- "win7" as go run paramter as follows.

```bash
go run ./extract.go -- "win7"
```

After moving the generated modules into the acmedns folder, you will also need to force override the version of the lego in the go.mod file under `src/`. by adding this line at the bottom of the go.mod file

```
replace github.com/go-acme/lego/v4 v4.16.1 => github.com/go-acme/lego/v4 v4.15.0

```

Make sure your Go version is on or below `1.20` for Windows 7 support.

### Module Usage

To use the module, you can call to the following function inside the `acmedns`

```go
func GetDNSProviderByJsonConfig(name string, js string)(challenge.Provider, error)

//For example
providersdef.GetDNSProviderByJsonConfig("gandi", "{\"Username\":\"far\",\"Password\":\"boo\"}")
```

This should be able to replace the default lego v4 build in one (the one attached below) that requires the use of environment variables

```go
// NewDNSChallengeProviderByName Factory for DNS providers.
func NewDNSChallengeProviderByName(name string) (challenge.Provider, error)
```
