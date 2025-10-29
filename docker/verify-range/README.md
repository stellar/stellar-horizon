# `stellar/horizon-verify-range`

This docker image allows running multiple instances of `horizon ingest verify-command` on a single machine or running it in [AWS Batch](https://aws.amazon.com/batch/).

## Data directory
The image by default stores all outputs of db, captive core, and buffered storage processes
under the `/data` directory at runtime in the container. Therefore it is strongly recommended
to provide an external volume mount to the container for `/data` of at least 300-500GB. Specify this at docker run time via - `docker run -v /host/volume:/data`

## Env variables

### Running locally

| Name     | Description                                           |
|----------|-------------------------------------------------------|
| `BRANCH` | Git branch to build (useful for testing PRs)          |
| `FROM`   | First ledger of the range (must be checkpoint ledger) |
| `TO`     | Last ledger of the range (must be checkpoint ledger)  |

### Running in AWS Batch

| Name                        | Description                                                          |
|-----------------------------|----------------------------------------------------------------------|
| `AWS_BATCH_JOB_ARRAY_INDEX` | The zero based index of a single batch Job.                          |
| `BATCH_START_LEDGER`        | The `FROM` ledger of the requested ledger range to verify.           |
| `BATCH_SIZE`                | Size of the batch, must be multiple of 64.                           |


### Datastore and GCP Credentials Usage

This image supports connecting to GCS buckets for ledger data instead of captive core. To use this feature configure the container with these additional settings:

#### GCP Credentials
- Purpose: To access GCS buckets the image needs GCP credentials. 
- Two options are available to provide this to container:
  - As an environment variable:
    - Pass the GCP JSON credentials as a string in a `GCP_CREDS` environment variable:
      ```sh
      docker run -e GCP_CREDS='{...}' ...
      ```
  - As a volume mount:
    - Mount the GCP json credentials file on host to the container, e.g.:
      ```sh
      docker run -v /host/path/credentials.json:/tmp/credentials.json -e GOOGLE_APPLICATION_CREDENTIALS=/tmp/credentials.json ...
      ```

#### GCS Datastore settings
- Purpose: Defines the GCS bucket name and ledger partioning used on the buckets. These settings are referenced as a single toml file at runtime. Here is an example [datastore_config.toml](../../../galexie/config.example.toml)
- Two options are available to provide this to container:
  - As an environment variable:
    - Pass the datastore TOML config as a string(including line breaks, tabs) in the `DATASTORE_CONFIG_PLAIN` environment variable:
      ```sh
      docker run -e DATASTORE_CONFIG_PLAIN='[buffered_storage_backend_config]\nbuffer_size = 5\n ...' 
      ```
  - As a volume mount:
    - Mount the datastore toml config file from host to the container, e.g.:
      ```sh
      docker run -v /host/path/datastore-config.toml:/tmp/datastore-config.toml -e DATASTORE_CONFIG=/tmp/datastore-config.toml 
      ```

### Examples of running container

#### Batch jobs example
When run from aws batch, given `BATCH_START_LEDGER=63` and `BATCH_SIZE=64`
it will generate runner jobs and give them each a  `AWS_BATCH_JOB_ARRAY_INDEX`. 
The verify-range container will then generate the associated ledger ranges per each job:

| `AWS_BATCH_JOB_ARRAY_INDEX` | `FROM` | `TO` |
|-----------------------------|--------|------|
| 0                           | 63     | 127  |
| 1                           | 127    | 191  |
| 2                           | 191    | 255  |
| 3                           | 255    | 319  |

#### `docker run` example
Running the verify-range image as local container with `docker run`
* run verify range and use captive core to get ledgers from pubnet:
  ```
  docker run -e FROM=63 \
                      -e TO=127 \
                      -e BRANCH=<target version> \
                      -e BASE_BRANCH=master \
                      verify-range:latest
  ```  
* run verify range with gcs datastore to use precomputed ledger metadata from buckets, captive core is not used:
  ```
  docker run -e FROM=63 \
                      -e TO=127 \
                      -e BRANCH=<target version> \
                      -e BASE_BRANCH=master \
                      -v /host/path/to/gcreds.json:/tmp/gcp.json \
                      -e GOOGLE_APPLICATION_CREDENTIALS=/tmp/gcp.json \
                      -v /host/path/to/datastore-config.tom:/tmp/datastore-config.toml \
                      -e DATASTORE_CONFIG=/tmp/datastore-config.toml \
                      verify-range:latest
  ```  
