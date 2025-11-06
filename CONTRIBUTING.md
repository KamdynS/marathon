# Contributing to Marathon

This project is my first foray into open source, so I am kind of approaching this in the way that I think will work best for myself. I'd love to standardize this as the repo gets more developed and finds it's style, but for now it will work the way that allows me to contribute the most to it. 

## Short background

I made this library mostly due to shortcomings in everything I could find online. At work, we needed some way to write backend services that used LLMs that was durable, but also fully fit within our own stack. We've used and are using some third party services, but we aim to get off them ASAP. With that in mind, the best solution to our problem, Temporal, just didn't seem viable. It was nice, but we wanted something that could run without needing to pay for temporal cloud, and something that could drop in nicely with our microservice architecture. 

With that in mind, my V1.0.0 of this project was a library that could give me everything I needed to rewrite a service previously for Node in this new library. All I did to get this project to where it is now is just implement whatever features I needed to get parity with our Node service in Go with durable workflows. 

## Plan for future featuers

While I do have a roadmap planned out, my immediate goal for tackling features is just to come up with a common backend service and begin writing that service using this library until the library is missing a feature, then write the feature. 

## How you can help

I'll start by opening all feature requests. Bugs and discussions may be opened by anyone, and anyone can pick up whichever issues you would like. 

## Testing Policy

- Prefer table-driven tests for all units where inputs/outputs can be enumerated.
- Add a test file for every package and for any file that has behavioral logic.
- Aim for at least 80% coverage across the repository.
- Keep tests race-safe: run `go test -race ./...` locally.
- Use subtests (`t.Run`) for clarity and to isolate cases.
- When adding new features, include coverage that exercises error paths and edge cases.

### Running tests and coverage

```bash
go test ./...
go test -race ./...
go test ./... -cover -coverprofile=coverage.out
go tool cover -func=coverage.out | sort -k3 -nr | head -n 30
go tool cover -html=coverage.out -o coverage.html
```

We keep CI fast; prefer unit tests over long-running integration tests by default. If you add an integration test, mark it with a build tag or `t.Skip` when external deps are unavailable.

