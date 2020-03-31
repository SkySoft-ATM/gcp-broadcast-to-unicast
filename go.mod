module github.com/skysoft-atm/gcp-broadcast-to-unicast

go 1.14

require (
	cloud.google.com/go v0.55.0
	github.com/lestrrat-go/backoff v1.0.0
	github.com/skysoft-atm/gorillaz v0.7.8
	github.com/skysoft-atm/supercaster v0.0.15
	go.uber.org/zap v1.10.0
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	google.golang.org/api v0.20.0
)

//replace github.com/skysoft-atm/supercaster => ../supercaster

//replace github.com/skysoft-atm/gorillaz => ../gorillaz
//replace github.com/skysoft-atm/consul-util => ../consul-util
