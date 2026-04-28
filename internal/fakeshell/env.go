package fakeshell

import "os"

// lookupEnv is wrapped so tests can stub it later if needed.
func lookupEnv(k string) (string, bool) { return os.LookupEnv(k) }
