# expect
Package expect is a Go version of the classic TCL Expect.

# Usage
example
```go
	// spawn `ssh 127.0.0.1` with default timeout=3s
	e, err := expect.Spawn("ssh 127.0.0.1", time.Second*3)
	if err != nil {
		panic(err)
	}

	// expect for `password:` with default timeout
	_, err = e.Expect("password:", -1)
	if err != nil {
		panic(err)
	}

	// send string with newline
	_, err = e.SendLine("passwd")
	if err != nil {
		panic(err)
	}

	// give control to the interactive user
	err = e.Interact()
	if err != nil {
		panic(err)
	}

	// wait for finish of spawned process
	err = e.Wait()
	if err != nil {
		panic(err)
	}
```