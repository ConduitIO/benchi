# Benchi

Benchi is a minimal benchmarking framework designed to help you measure the
performance of your applications and infrastructure. It leverages Docker to
create isolated environments for running benchmarks and collecting metrics.

It was developed to simplify the process of setting up and running benchmarks
for [Conduit](https://github.com/ConduitIO/conduit).

![demo](https://github.com/user-attachments/assets/334d854d-0466-489c-bff6-95f4471b457f)

## Features

- **Docker Integration**: Define and manage your benchmarking environments using
  Docker Compose.
- **Metrics Collection**: Collect and export metrics in CSV format for further
  analysis.
- **Custom Hooks**: Define custom hooks to run commands at various stages of the
  benchmark.
- **Progress Monitoring**: Real-time monitoring of container statuses and
  metrics during the benchmark run.

## Installation

To install Benchi, download
the [latest release](https://github.com/ConduitIO/benchi/releases/latest) or
install it using Go:

```sh
go install github.com/conduitio/benchi/cmd/benchi@latest
```

## Usage

### Running Benchmarks

To run the example benchmark, use the following command:

```sh
benchi -config ./example/bench-kafka-kafka/bench.yaml
```

### Command-Line Flags

- `-config`: Path to the benchmark config file (required).
- `-out`: Path to the output folder (default: `./results/${now}`*).
- `-tool`: Filter tool to be tested (can be provided multiple times).
- `-tests`: Filter test to run (can be provided multiple times).

\* `${now}` is replaced with the current time formatted as `YYYYMMDDHHMMSS`.

### Docker network

Benchi creates a Docker network named `benchi` to connect the infrastructure
services and tools. This network is created automatically and removed after the
benchmark run. Please make sure to connect your services to this network to
ensure they can communicate with each other.

Example Docker Compose configuration:

```yaml
services:
  my-service:
    networks:
      - benchi

networks:
  benchi:
    external: true
```

### Configuration

Benchi uses a YAML configuration file to define the benchmark in combination
with Docker Compose configurations.

Below is an example configuration:

```yaml
infrastructure:
  database:
    compose: "./compose-database.yml"
  cache:
    compose: "./compose-cache.yml"

tools:
  my-app:
    compose: "./compose-my-app.yml"

metrics:
  prometheus:
    collector: "prometheus"
    settings:
      url: "http://localhost:9090/metrics"
      queries:
        - name: "http_requests_rate"
          query: "rate(request_count{endpoint=hello}[2s])"
          unit: "req/s"
          interval: "1s"

tests:
  - name: Endpoint Load
    duration: 2m
    steps:
      pre-infrastructure:
      post-infrastructure:
        - name: Setup Database
          container: database
          run: /scripts/setup-database.sh
      pre-tools:
      post-tools:
      pre-test:
      during:
        - name: Run Load Test
          container: my-app
          run: /scripts/run-load-test.sh
      post-test:
      pre-cleanup:
        - name: Cleanup
          container: my-app
          run: /scripts/cleanup.sh
      post-cleanup:
```

#### `infrastructure`

The `infrastructure` section defines the Docker Compose configurations for the
infrastructure services required for the benchmark. Each service is identified
by a custom name, used in logging and to correlate overridden configurations
specified in a test (see [tests](#tests)). The path to the docker compose file
is relative to the location of the benchmark configuration file.

Example:

```yaml
infrastructure:
  name-of-infrastructure-service:
    compose: "./path/to/compose-file.yml"
```

#### `tools`

The `tools` section defines the Docker Compose configurations for the tools
being benchmarked. Each tool is identified by a custom name, used in logging and
to correlate overridden configurations specified in a test (see [tests](#tests).
The path to the docker compose file is relative to the location of the benchmark
configuration file.

Example:

```yaml
tools:
  name-of-tool:
    compose: "./path/to/compose-file.yml"
```

#### `metrics`

The `metrics` section defines the metric collectors running during the
benchmark. Each metric collector has a custom name used for logging. The
`collector` field specifies the type of metric collector to use. The `settings`
field contains the configuration for the chosen collector.

Example:

```yaml
metrics:
  name-of-metric-collector:
    collector: "conduit"
    settings:
      url: "http://localhost:8080/metrics"
```

> [!NOTE]  
> Metrics collectors run in the benchi process, which runs outside of docker
> on the host machine. Ensure that the metric collector can access the
> endpoints of the services being benchmarked by exposing the necessary ports
> in the Docker Compose configurations.

See [collectors](#collectors) for available collectors and their configurations.

#### `tests`

The `tests` section defines the benchmarks to run. Each test has a custom name
used for logging. The `duration` field specifies the duration of the test. The
`steps` field contains the commands to run at various stages of the benchmark.

The `steps` field contains the following stages:

- `pre-infrastructure`: Commands to run before starting the infrastructure
  services.
- `post-infrastructure`: Commands to run after starting the infrastructure
  services.
- `pre-tools`: Commands to run before starting the tools.
- `post-tools`: Commands to run after starting the tools.
- `pre-test`: Commands to run before starting the test.
- `during`: Commands to run during the test.
- `post-test`: Commands to run after the test.
- `pre-cleanup`: Commands to run before cleaning up the test.
- `post-cleanup`: Commands to run after cleaning up the test.

> [!NOTE]
> Steps are generally executed sequentially and in the order specified in the
> configuration. However, the `during` step is an exception, as all commands
> under this step are executed concurrently and will run for the duration of the
> test.

Each hook can run its commands either in an existing container or in a temporary
container created from a specified image. The `container` field specifies the
name of the container to run the commands in. The `image` field specifies the
image to use for the temporary container. If neither `container` nor `image` is
specified, the commands will run in a temporary container using the
`alpine:latest` image. Note that running a custom script in a container requires
the container to contain the necessary script (hint: mount the script as a
volume).

You can optionally configure the `tools` field to run a hook only for certain
tools. If the field is not present or empty, the hook is applied for all tools.

Example:

```yaml
tests:
  - name: My Test
    duration: 2m
    steps:
      pre-infrastructure:
      post-infrastructure:
        # This script will run after the infrastructure services have started.
        # It will run in the `database` container (presumably started by the
        # infrastructure service).
        - name: Setup Database
          container: database
          run: /scripts/setup-database.sh
      pre-tools:
      post-tools:
      pre-test:
      during:
        # This script will run for the duration of the test (2 minutes). It will
        # run in the `my-app` container (presumably started by the tool). The
        # script will receive a SIGINT signal when the test duration is over.
        - name: Run Load Test
          container: my-app
          tools:
            - my-app # Only run this hook when benchmarking my-app
          run: /scripts/run-load-test.sh
      post-test:
      pre-cleanup:
        # This script will run before infrastructure and tools containers are
        # stopped and removed. It will run in a temporary container created
        # from the `busybox:latest` image and connected to the `benchi` network.
        - name: Cleanup
          image: "busybox:latest"
          run: |
            echo "Cleaning up..."
            sleep 5
      post-cleanup:
```

You can also include custom `infrastructure` and `tools` configurations to
override the default configurations specified in the `infrastructure` and
`tools` sections. Note that the global configurations will still be applied, the
additional configurations are merged with the global configurations (see
[merging compose files](https://docs.docker.com/compose/how-tos/multiple-compose-files/merge/)).
This can be useful to inject custom configurations for a specific test.

> [!IMPORTANT]
> Since the provided configurations are merged with the global configurations,
> any paths specified in the custom docker compose configurations should be
> relative to the location of the global docker compose configuration. See
> [merging rules](https://docs.docker.com/compose/how-tos/multiple-compose-files/merge/#merging-rules).

Example:

```yaml
tests:
  - name: My Test
    duration: 2m
    infrastructure:
      name-of-infrastructure-service:
        compose: "./compose-file-infra.override.yml"
    tools:
      name-of-tool:
        compose: "./compose-file-tool.override.yml"
```

## Collectors

Collectors are used to collect metrics from various sources during the benchmark
run. The collected metrics are exported in CSV format for further analysis.

Collectors are configured in the [`metrics` section](#metrics) of the benchmark
configuration file.

Supported collectors:

- [Conduit](#conduit)
- [Docker](#docker)
- [Kafka](#kafka)
- [Prometheus](#prometheus)

### Conduit

The Conduit metrics collector tracks the throughput for each configured
pipeline in [Conduit](https://github.com/conduitio/conduit).

Settings:

- `url`: URL of the Conduit metrics endpoint (needs to be reachable from the
  benchi process).
- `pipelines`: Array of pipelines to track.

```yaml
metrics:
  my-conduit-collector:
    collector: "conduit"
    settings:
      url: "http://localhost:8080/metrics"
      pipelines:
        - "pipeline1"
        - "pipeline2"
```

Metrics:

- `msg-rate-per-second[PIPELINE]`: Messages per second per pipeline.
- `msg-megabytes-in-per-second[PIPELINE]`: Incoming megabytes per second per
  pipeline (measured as records read by the source connector).
- `msg-megabytes-out-per-second[PIPELINE]`: Outgoing megabytes per second per
  pipeline (measured as records written by the destination connector).

### Docker

The Docker metrics collector tracks the container resource usage during the
benchmark run.

Settings:

- `containers`: Array of containers to track.

```yaml
metrics:
  my-docker-collector:
    collector: "docker"
    settings:
      containers:
        - "my-app"
        - "database"
        - "cache"
```

Metrics:

- `cpu-percentage[CONTAINER]`: CPU usage in percent at a specific point in time
  per container.
- `memory-usage[CONTAINER]`: Memory usage in megabytes at a specific point in
  time per container.

### Kafka

The Kafka metrics collector tracks the throughput for each configured topic in
[Apache Kafka](https://kafka.apache.org/).

The collector expects Kafka to expose a Prometheus metrics endpoint via the
[Prometheus JMX exporter](https://prometheus.github.io/jmx_exporter/). To see
how to configure it, check out the
[example configuration](./example/infra/compose-kafka.yml).

Settings:

- `url`: URL of the Kafka metrics endpoint (needs to be reachable from the
  benchi process).
- `topics`: Array of topics to track.

```yaml
metrics:
  my-kafka-collector:
    collector: "kafka"
    settings:
      url: "http://localhost:7071/metrics"
      topics:
        - "topic1"
        - "topic2"
```

Metrics:

- `msg-rate-in-per-second[TOPIC]`: Incoming messages per second per topic.
- `msg-megabytes-in-per-second[TOPIC]`: Incoming megabytes per second per topic.
- `msg-megabytes-out-per-second[TOPIC]`: Outgoing megabytes per second per topic.

### Prometheus

The Prometheus metrics collector continuously scrapes a metrics endpoint, stores
the metrics in memory and queries them using
[PromQL](https://prometheus.io/docs/prometheus/latest/querying/basics/).

It is expected that the query returns a matrix with a single series. The query
should be a
[ranged query](https://prometheus.io/docs/prometheus/latest/querying/basics/#range-vector-selectors),
which will be evaluated between the start of the test and the end of the test.

Settings:

- `url`: URL of the Prometheus metrics endpoint.
- `queries`: Array of queries to run.
   - `name`: Name of the query.
   - `query`: [PromQL](https://prometheus.io/docs/prometheus/latest/querying/basics/) query.
   - `unit`: Unit of the query (optional, only for displaying in the CLI).
   - `interval`: Resolution of the ranged query.

```yaml
metrics:
  my-prometheus-collector:
    collector: "prometheus"
    settings:
      url: "http://localhost:8080/metrics"
      queries:
        - name: "http_request_success_rate"
          query: "rate(request_count{endpoint=hello,status=200}[2s])"
          unit: "req/s"
          interval: "1s"
        - name: "http_request_fail_rate"
          query: "rate(request_count{endpoint=hello,status!=200}[2s])"
          unit: "req/s"
          interval: "1s"
```

Metrics are user defined using the `queries` field. The `name` field is used as
the metric name in the CSV output.

## Troubleshooting

- Benchi fails with the error `Cannot connect to the Docker daemon at
  unix:///var/run/docker.sock. Is the docker daemon running?`.

Benchi is communicating with Docker using the default Docker socket. If you are
using Docker Desktop, you can enable it under Settings -> Advanced and check the
box for "Allow the default Docker socket to be used".

## License

Benchi is licensed under the Apache License, Version 2.0. See the
[`LICENSE`](./LICENSE.md) file for more details.
