# Benchi example

This folder includes an example of how to use Benchi. We use the following
folder structure:

- `infra`: Contains the Docker compose files for any infrastructure needed for
  the benchmarks. These are supposed to contain configurations that are common
  to all benchmarks. The specifics will be overridden by the benchmarks
  themselves.
- `tools`: Contains the Docker compose files for the tools being benchmarked.
  Same as `infra`, these are supposed to contain configurations common to all
  benchmarks.
- `bench-kafka-kafka`: Contains a specific benchmark configuration
  ([`bench.yml`](./bench-kafka-kafka/bench.yml)) for testing a kafka->kafka
  data pipeline. It also includes any overrides for the `infra` and `tools`
  configurations.

You can run the example benchmark by running the following command:

```sh
benchi -config example/bench-kafka-kafka/bench.yml
```

Running this benchmark will start a Kafka instance, create two test topics
(`benchi-in` and `benchi-out`), produce test messages to one topic and then
start a Conduit data pipeline configured to consume messages from the first
topic and produce them to the second topic. The benchmark will run for 1 minute.
While the benchmark is running, benchi will collect metrics from Kafka, Conduit
and Docker, displaying them in the CLI. After the benchmark is complete, the
metrics will be exported to CSV files in the output folder (e.g.
`results/20060102_150405`).

Benchi will store the results in the `results` folder. Inside the results folder,
you will find a folder named after the current date and time (e.g.
`results/20060102_150405`). This folder will contain logs and results:

- `benchi.log`: Log file containing the output of the benchmark run.
- `infra.log`: Log file containing the output of the infrastructure docker containers.
- `tools.log`: Log file containing the output of the tools docker containers.
- `conduit.csv`: Metrics collected using the [Conduit](../README.md#conduit) collector.
- `docker.csv`: Metrics collected using the [Docker](../README.md##docker) collector.
- `kafka.csv`: Metrics collected using the [Kafka](../README.md##kafka) collector.
