// add file in v.1.0.3
// kvEntity.go is file that declare entities for value in KV

package consul

// entity about redis connection config KV
type RedisConfigKV struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	DB   int    `json:"DB"`
}
