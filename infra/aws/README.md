# Deploy

## Local (Docker, whole stack + client)

```sh
# once: .env with random secrets + a fresh executor key (container, since macOS LibreSSL lacks ed25519)
docker run --rm -v "$PWD:/w" python:3.13-slim python /w/infra/aws/ansible/files/init_env.py /w
docker compose -f docker-compose.yml -f infra/client.compose.yml up -d --build --wait
make smoke    # client on http://localhost:${CLIENT_PORT:-3000}; set CORS_ORIGINS to match
```

## AWS test host

A single t4g.small instance (arm64, Ubuntu 24.04) runs `docker-compose.yml` + `infra/client.compose.yml` + `compose.aws.yml` (Caddy, auto HTTPS).
The images are built on the Mac (arm64, same as the instance) and streamed over SSH. The host never builds anything.
URLs: `https://<ip>.sslip.io` for the app, with `api.`, `md.` (market-data + WS) and `history.` as subdomains. No DNS setup needed.
The first run creates `/opt/nearly/.env` on the host with its own secrets and executor key. Later runs keep it.

```sh
cd infra/aws/terraform && terraform init && terraform apply     # AWS profile `crayo`; writes ../ansible/inventory.ini
cd ../ansible && ansible-playbook playbook.yml                  # build, ship, start, HTTPS smoke test; rerun to redeploy
terraform -chdir=../terraform destroy                           # tear down
```

Reuse an existing bot key: `-e executor_private_key=ed25519:... -e executor_public_key=ed25519:...`.
