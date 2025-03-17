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

The output folder will contain logs and results:

- `benchi.log`: Log file containing the output of the benchmark run.
- `aggregated-results.csv`: Aggregated metric results from all collectors.
- `kafka-to-kafka_conduit`: Folder containing the logs and metrics for the
  `kafka-to-kafka` test and the `conduit` tool.
  - `infra_kafka.log`: Log file containing the output of the kafka infrastructure
    docker containers.
  - `tools.log`: Log file containing the output of the tools docker containers
    (conduit).
  - `conduit.csv`: Metrics collected using the [Conduit](../README.md#conduit) collector.
  - `docker.csv`: Metrics collected using the [Docker](../README.md##docker) collector.
  - `kafka.csv`: Metrics collected using the [Kafka](../README.md##kafka) collector.
