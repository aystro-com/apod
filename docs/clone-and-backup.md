# Cloning & backups

apod has two distinct ways to reproduce a site. They use deliberately different
mechanisms because they solve different problems.

## Clone — `apod clone <source> <target>`

A live, same-host copy of a site ("spin up staging from prod").

**Mechanism: consistent physical volume copy.**

1. Quiesce the source (stop its containers) so its files and databases are at a
   consistent, at-rest state.
2. Copy the site's files and data volumes byte-for-byte — preserving mode,
   ownership and symlinks.
3. Bring the source back up.
4. Stand the target up on the copied data, **preserving the source's database
   name and credentials**, so the copied data directory is valid as-is.

**Why this design.** Cloning must not depend on the stack. A physical copy of a
volume is identical for MySQL, Postgres, Mongo, Redis, SQLite — or a site with no
database at all. There is no per-engine dump/restore code, so there is nothing
stack-specific to break. This replaced an earlier logical dump/restore that grew
a new failure mode for every database engine (and corrupted its own dumps via the
Docker exec stream).

The credentials are reused on purpose: a site's database listens only on its
private per-site network, and the app container in that network already holds the
password, so a unique-per-site password is not a security boundary — and
regenerating it would invalidate the copied data directory.

**Trade-off.** The source is briefly down for the duration of the copy. A
filesystem snapshot (ZFS/btrfs/LVM) would remove even that — it is the natural
fast path and the place to optimise next.

Compose-managed sites are the exception: they provision their own networks and
secrets, so they clone declared file paths plus a logical dump/restore of
declared databases.

## Backup / restore — `apod backup …`

Point-in-time recovery and **portable** copies (local, S3/R2, SFTP; potentially a
different host or architecture).

**Mechanism: logical dump.** Databases are captured with `mysqldump` / `pg_dump`
(stdout only, demultiplexed from the Docker exec stream so the SQL is not
corrupted by frame headers or stderr). Files are archived alongside.

`apod backup new-site <source> <id> <new>` provisions a new site from a stored
backup. It rebuilds the database from the logical dump (never the raw data
directory — a hot copy is not crash-consistent) while preserving the source's
credentials so the app, its restored `.env` and the new database stay consistent.

## Which to use

| Goal | Use | Why |
|------|-----|-----|
| Staging from current prod, same host | `apod clone` | fast, exact, stack-agnostic |
| Restore a historical point in time | `apod backup new-site` / `restore` | dumps are versioned snapshots |
| Move a site to another server / keep offsite | backups | logical dumps are portable |
