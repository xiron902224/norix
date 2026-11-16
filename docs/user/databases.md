# Databases

**Databases** in the UI manages **platform-managed database instances** for your org (provisioning depends on your Norix version and node capabilities).

Typical flow:

1. Open **Databases**
2. Create an instance (engine/size as offered)
3. Copy connection details into your workload env vars
4. Deploy / restart the app so it picks up the DSN

Back up application data via **Ops** backups when available, and always keep an off-box copy of production secrets.

> Operators: Postgres for the control plane itself is separate from app databases — see install docs.
