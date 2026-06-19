package sql

type Driver string

const (
	DriverPgsql Driver = "pgsql"
)

type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DbName   string
}
