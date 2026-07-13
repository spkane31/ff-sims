"""
Shared Temporal Cloud / local-dev client factory for the ESPN worker package.

Used by both worker.py (the long-running Temporal worker) and
register_league.py (the one-shot CLI) so there's a single implementation of
the TLS cert-chain handling Temporal Cloud's custom-CA endpoints need.

Temporal Cloud env vars:
  TEMPORAL_NAMESPACE_ENDPOINT     e.g. ff-sims.b3i2g.tmprl-test.cloud:7233
  TEMPORAL_NAMESPACE              e.g. ff-sims.b3i2g
  TEMPORAL_API_KEY                API key

Local dev server fallback:
  TEMPORAL_HOST                default localhost:7233
  TEMPORAL_NAMESPACE           default "default"
"""
import os
import re
import subprocess

from temporalio.client import Client
from temporalio.service import TLSConfig


def _fetch_server_tls_config(endpoint: str) -> TLSConfig:
    """Trust whatever cert chain the server presents — equivalent to InsecureSkipVerify=true in Go.

    Uses `openssl s_client -showcerts` to capture every cert in the chain (leaf,
    intermediates, and root CA). Passing the full chain as server_root_ca_cert lets
    rustls build a valid path even when the CA is not in the system trust store, which
    is the common case for tmprl-test.cloud and other custom-CA Temporal environments.

    Python 3.12's ssl module only exposes the leaf cert via getpeercert(), which fails
    as a rustls trust anchor because its CA bit is false. This approach works on any
    Python version and requires openssl to be installed in the container.
    """
    host, port_str = endpoint.rsplit(":", 1)
    result = subprocess.run(
        ["openssl", "s_client", "-connect", f"{host}:{port_str}", "-showcerts"],
        input=b"",
        capture_output=True,
        timeout=10,
    )
    pem_certs = re.findall(
        rb"-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----",
        result.stdout,
        re.DOTALL,
    )
    if not pem_certs:
        raise RuntimeError(
            f"Could not retrieve TLS certificate chain from {endpoint} — "
            "is the endpoint reachable and is openssl installed in the container?"
        )
    return TLSConfig(server_root_ca_cert=b"\n".join(pem_certs))


async def create_client() -> Client:
    if endpoint := os.getenv("TEMPORAL_NAMESPACE_ENDPOINT"):
        return await Client.connect(
            endpoint,
            namespace=os.environ["TEMPORAL_NAMESPACE"],
            tls=_fetch_server_tls_config(endpoint),
            api_key=os.getenv("TEMPORAL_API_KEY"),
        )
    return await Client.connect(
        os.getenv("TEMPORAL_HOST", "localhost:7233"),
        namespace=os.getenv("TEMPORAL_NAMESPACE", "default"),
    )
