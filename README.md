# Metroid Prime: Federation Force and Metroid Prime: Blast Ball replacement server
Includes both the authentication and secure servers

## Compiling

### Setup
Install [Go](https://go.dev/doc/install) and [git](https://git-scm.com/downloads), then clone and enter the repository

```bash
$ git clone https://github.com/PretendoNetwork/metroid-prime-federation-force
$ cd metroid-prime-federation-force
```

### Compiling and running using `docker` (PREFERRED)
Make sure you have Docker installed on your system. This can be done using various instructions available online.

Once installed, execute the following to build:

```bash
$ docker build -t mpff --build-arg BUILD_STRING=YOUR_BUILD_STRING_HERE .
$ docker image prune --filter label=stage=builder -f
```
Note: `--build-arg` flag/variable is optional.

Create a `.env` file with all of the necessary environment variables set. The variable list is available below.

Example:
```
PN_MPFF_KERBEROS_PASSWORD=yourpassword
PN_MPFF_AUTHENTICATION_SERVER_PORT=61001
...
```

Then, you can use the following command to run the image.
```bash
$ docker run --name mpff --env-file .env -it mpff
```

Other tools and systems can also make use of this image, including Docker Compose and Portainer.

### Compiling using `go`
To compile using Go, `go get` the required modules and then `go build` to your desired location. You may also want to tidy the go modules, though this is optional

```bash
$ go get -u
$ go mod tidy
$ go build -o build/metroid-prime-federation-force
```

The server is now built to `build/metroid-prime-federation-force`

When compiling with only Go, the authentication servers build string is not automatically set. This should not cause any issues with gameplay, but it means that the server build will not be visible in any packet dumps or logs a title may produce

To compile the servers with the authentication server build string, add `-ldflags "-X 'main.serverBuildString=BUILD_STRING_HERE'"` to the build command, or use `make` to compile the server

### Compiling using `make`
Compiling using `make` will read the local `.git` directory to create a dynamic authentication server build string, based on your repositories remote origin and current commit. It will also use the current folders name as the executables name

Install `make` onto your system (this varies by OS), and run `make` while inside the repository

```bash
$ make
```

The server is now built to `build/metroid-prime-federation-force` with the authentication server build string already set

## Configuration
All configuration options are handled via environment variables

`.env` files are supported

| Name                                 | Description                                                                                                         | Required                            |
|--------------------------------------|---------------------------------------------------------------------------------------------------------------------|-------------------------------------|
| `PN_MPFF_POSTGRES_URI`               | Fully qualified URI to your Postgres server (Example `postgres://username:password@localhost/mpff?sslmode=disable`) | Yes                                 |
| `PN_MPFF_AES_KEY`                    | AES key used in tokens provided by the account server                                                               | Yes                                 |
| `PN_MPFF_AUTHENTICATION_SERVER_PORT` | Port for the authentication server                                                                                  | Yes                                 |
| `PN_MPFF_SECURE_SERVER_HOST`         | Host name for the secure server (should point to the same address as the authentication server)                     | Yes                                 |
| `PN_MPFF_SECURE_SERVER_PORT`         | Port for the secure server                                                                                          | Yes                                 |
| `PN_MPFF_ACCOUNT_GRPC_HOST`          | Host name for your account server gRPC service                                                                      | Yes                                 |
| `PN_MPFF_ACCOUNT_GRPC_PORT`          | Port for your account server gRPC service                                                                           | Yes                                 |
| `PN_MPFF_ACCOUNT_GRPC_API_KEY`       | API key for your account server gRPC service                                                                        | No (Assumed to be an open gRPC API) |
| `PN_MPFF_FRIENDS_GRPC_HOST`          | Host name for your friends server gRPC service                                                                      | Yes                                 |
| `PN_MPFF_FRIENDS_GRPC_PORT`          | Port for your friends server gRPC service                                                                           | Yes                                 |
| `PN_MPFF_FRIENDS_GRPC_API_KEY`       | API key for your friends server gRPC service                                                                        | No (Assumed to be an open gRPC API) |
