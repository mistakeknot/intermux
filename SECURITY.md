# Security

Report vulnerabilities privately through GitHub's "Report a vulnerability" button on this repository, or by email to the address on the maintainer's GitHub profile. You will get an acknowledgement within a week.

## Threat model

intermux reads the screen contents of every tmux session on the socket it is pointed at and exposes them through `peek_agent`, `search_output`, and `activity_feed`. Run it only on a machine where every tmux user is trusted, or point it at a private socket with `TMUX_SOCKET`. Agent correlation files live in a per-user, owner-only directory (`INTERMUX_MAPPING_DIR`, default `~/.local/state/intermux/mappings`); the server refuses a directory that is group- or world-accessible.

intermux's metadata pushes to intermute are identified by `X-Agent-ID` only, with no accompanying token. They are therefore trusted at the loopback level, not authenticated per agent: any local process that can reach intermute's port can claim to be pushing on behalf of a given agent ID. This is the same loopback trust boundary intermute itself documents — see its README § Auth.
