package enum

const (
	EnvironmentDevelopment Environment = "dev"
	EnvironmentProduction  Environment = "prod"
)

type Environment string

func (e Environment) String() string {
	return string(e)
}
