#/bin/sh

set -e -x

mount -t nfsd nfsd /proc/fs/nfsd
mount -t rpc_pipefs sunrpc /var/lib/nfs/rpc_pipefs

echo "/var/files *(rw,sync,no_subtree_check,fsid=0,no_root_squash,insecure)" > /etc/exports

rpcbind -w
rpc.mountd
RPC_STATD_NO_NOTIFY=1 rpc.statd
rpc.idmapd
exportfs -r
rpc.nfsd 8
echo no more logs, just waiting now
sleep infinity
