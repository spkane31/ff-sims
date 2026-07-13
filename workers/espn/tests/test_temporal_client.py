import temporal_client
import worker


def test_worker_reuses_shared_create_client():
    """worker.py must not define its own copy of create_client — both modules
    share the same implementation, or the TLS cert-chain handling for
    Temporal Cloud's custom-CA endpoints would drift between them."""
    assert worker.create_client is temporal_client.create_client
