$ErrorActionPreference = "Stop"
$compose = @("--project-name", "pc-rollout-$PID", "-f", "examples/rollout/compose.yml")
$services = @("rollout-old", "rollout-new", "rollout-major", "rollout-bridge", "rollout-client")

try {
    docker compose @compose build @services
    if ($LASTEXITCODE -ne 0) { throw "Rollout image build failed" }

    docker compose @compose up -d rollout-old rollout-new rollout-major rollout-bridge
    if ($LASTEXITCODE -ne 0) { throw "Rollout servers failed to start" }

    docker compose @compose run --rm -T rollout-client
    if ($LASTEXITCODE -ne 0) { throw "Rollout compatibility scenarios failed" }
}
finally {
    docker compose @compose down --remove-orphans
}
