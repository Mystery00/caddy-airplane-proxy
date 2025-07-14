package caddy_airplane_proxy

import "os"

func checkExist(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}
