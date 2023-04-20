#!/usr/bin/python
"""
Copyright 2019 The Ceph-CSI Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
#pylint: disable=line-too-long
python tool to trace backend image name from pvc
Note: For the script to work properly python>=3.x is required
sample input:
python -c oc -k /home/.kube/config -n default -rn rook-ceph -id admin -key
adminkey -cm ceph-csi-config
Sample output:
+------------------------------------------------------------------------------------------------------------------------------------------------------------+
|                                                                            RBD                                                            |
+----------+------------------------------------------+----------------------------------------------+-----------------+--------------+------------------+
| PVC Name |                 PV Name                  |                  Image
Name                  | PV name in omap | Image ID in omap | Image in cluster |
+----------+------------------------------------------+----------------------------------------------+-----------------+--------------+------------------+
| rbd-pvc  | pvc-f1a501dd-03f6-45c9-89f4-85eed7a13ef2 | csi-vol-1b00f5f8-b1c1-11e9-8421-9243c1f659f0 |       True      |     True     |      False       |
| rbd-pvcq | pvc-09a8bceb-0f60-4036-85b9-dc89912ae372 | csi-vol-b781b9b1-b1c5-11e9-8421-9243c1f659f0 |       True      |     True     |       True       |
+----------+------------------------------------------+----------------------------------------------+-----------------+--------------+------------------+
+--------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
|                                                                                  CephFS                                                                                  |
+----------------+------------------------------------------+----------------------------------------------+-----------------+----------------------+----------------------+
|    PVC Name    |                 PV Name                  |                Subvolume Name                | PV name in omap | Subvolume ID in omap | Subvolume in cluster |
+----------------+------------------------------------------+----------------------------------------------+-----------------+----------------------+----------------------+
| csi-cephfs-pvc | pvc-b3492186-73c0-4a4e-a810-0d0fa0daf709 | csi-vol-6f283b82-a09d-11ea-81a7-0242ac11000f |       True      |         True         |         True         |
+----------------+------------------------------------------+----------------------------------------------+-----------------+----------------------+----------------------+
"""

import argparse
import subprocess
import json
import sys
import re
import prettytable
PARSER = argparse.ArgumentParser()

# -p pvc-test -k /home/.kube/config -n default -rn rook-ceph
PARSER.add_argument("-p", "--pvcname", default="", help="PVC name")
PARSER.add_argument("-c", "--command", default="oc",
                    help="kubectl or oc command")
PARSER.add_argument("-k", "--kubeconfig", default="",
                    help="kubernetes configuration")
PARSER.add_argument("-n", "--namespace", default="default",
                    help="namespace in which pvc created")
PARSER.add_argument("-t", "--toolboxdeployed", type=bool, default=True,
                    help="is rook toolbox deployed")
PARSER.add_argument("-d", "--debug", type=bool, default=False,
                    help="log commands output")
PARSER.add_argument("-rn", "--rooknamespace",
                    default="rook-ceph", help="rook namespace")
PARSER.add_argument("-id", "--userid",
                    default="admin", help="user ID to connect to ceph cluster")
PARSER.add_argument("-key", "--userkey",
                    default="", help="user password to connect to ceph cluster")
PARSER.add_argument("-cm", "--configmap", default="ceph-csi-config",
                    help="configmap name which holds the cephcsi configuration")
PARSER.add_argument("-cmn", "--configmapnamespace", default="default",
                    help="namespace where configmap exists")


def list_pvc_vol_name_mapping(arg):
    """
    list pvc and volume name mapping
    """
    table_rbd = prettytable.PrettyTable()
    table_rbd.title = "RBD"
    table_rbd.field_names = ["PVC Name", "PV Name", "Image Name", "PV name in omap",
                             "Image ID in omap", "Image in cluster"]

    table_cephfs = prettytable.PrettyTable()
    table_cephfs.title = "CephFS"
    table_cephfs.field_names = ["PVC Name", "PV Name", "Subvolume Name", "PV name in omap",
                                "Subvolume ID in omap", "Subvolume in cluster"]

    cmd = [arg.command]

    if arg.kubeconfig != "":
        if arg.command == "oc":
            cmd += ["--config", arg.kubeconfig]
        else:
            cmd += ["--kubeconfig", arg.kubeconfig]
    cmd += ["--namespace", arg.namespace]
    if arg.pvcname != "":
        cmd += ['get', 'pvc', arg.pvcname, '-o', 'json']
        # list all pvc and get mapping
    else:
        cmd += ['get', 'pvc', '-o', 'json']

    with subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT) as out:
        stdout, stderr = out.communicate()

    if stderr is not None:
        if arg.debug:
            print("failed to list pvc %s", stderr)
        sys.exit()
    try:
        pvcs = json.loads(stdout)
    except ValueError as err:
        print(err, stdout)
        sys.exit()
    format_and_print_tables(arg, pvcs, table_rbd, table_cephfs)

def format_and_print_tables(arg, pvcs, table_rbd, table_cephfs):
    """
    format and print tables with all relevant information.
    """
    if arg.pvcname != "":
        pvname = pvcs['spec']['volumeName']
        pvdata = get_pv_data(arg, pvname)
        if is_rbd_pv(arg, pvname, pvdata):
            format_table(arg, pvcs, pvdata, table_rbd, True)
        else:
            format_table(arg, pvcs, pvdata, table_cephfs, False)
    else:
        for pvc in pvcs['items']:
            pvname = pvc['spec']['volumeName']
            pvdata = get_pv_data(arg, pvname)
            if is_rbd_pv(arg, pvname, pvdata):
                format_table(arg, pvc, pvdata, table_rbd, True)
            else:
                format_table(arg, pvc, pvdata, table_cephfs, False)
    print(table_rbd)
    print(table_cephfs)

#pylint: disable=too-many-locals
def format_table(arg, pvc_data, pvdata, table, is_rbd):
    """
    format tables for pvc and image information
    """
    # pvc name
    pvcname = pvc_data['metadata']['name']
    # get pv name
    pvname = pvc_data['spec']['volumeName']
    # get volume handler from pv
    volume_name = get_volume_handler_from_pv(arg, pvname)
    # get volume handler
    if volume_name == "":
        table.add_row([pvcname, "", "", False,
                       False, False])
        return
    pool_name = get_pool_name(arg, volume_name, is_rbd)
    if pool_name == "":
        table.add_row([pvcname, pvname, "", False,
                       False, False])
        return
    # get image id
    image_id = get_image_uuid(volume_name)
    if image_id is None:
        table.add_row([pvcname, pvname, "", False,
                       False, False])
        return
    # get volname prefix
    volname_prefix = get_volname_prefix(arg, pvdata)
    # check image/subvolume details present rados omap
    pv_present, uuid_present = validate_volume_in_rados(arg, image_id, pvname, pool_name, is_rbd)
    present_in_cluster = False
    if is_rbd:
        present_in_cluster = check_image_in_cluster(arg, image_id, pool_name, volname_prefix)
    else:
        fsname = get_fsname_from_pvdata(arg, pvdata)
        subvolname = volname_prefix + image_id
        present_in_cluster = check_subvol_in_cluster(arg, subvolname, fsname)
    image_name = volname_prefix + image_id
    table.add_row([pvcname, pvname, image_name, pv_present,
                   uuid_present, present_in_cluster])

def validate_volume_in_rados(arg, image_id, pvc_name, pool_name, is_rbd):
    """
    validate volume information in rados
    """
    pv_present = check_pv_name_in_rados(arg, image_id, pvc_name, pool_name, is_rbd)
    uuid_present = check_image_uuid_in_rados(arg, image_id, pvc_name, pool_name, is_rbd)
    return pv_present, uuid_present

def check_pv_name_in_rados(arg, image_id, pvc_name, pool_name, is_rbd):
    """
    validate pvc information in rados
    """
    omapkey = f'csi.volume.{pvc_name}'
    cmd = ['rados', 'getomapval', 'csi.volumes.default',
           omapkey, "--pool", pool_name]
    if not arg.userkey:
        cmd += ["--id", arg.userid, "--key", arg.userkey]
    if not is_rbd:
        cmd += ["--namespace", "csi"]
    if arg.toolboxdeployed is True:
        kube = get_cmd_prefix(arg)
        cmd = kube + cmd
    with subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT) as out:
        stdout, stderr = out.communicate()

    if stderr is not None:
        return False
    name = b''
    lines = [x.strip() for x in stdout.split(b"\n")]
    for line in lines:
        if b' ' not in line:
            continue
        if b'value' in line and b'bytes' in line:
            continue
        part = re.findall(br'[A-Za-z0-9\-]+', line)
        if part:
            name += part[-1]
    if name.decode() != image_id:
        if arg.debug:
            decoded_name = name.decode()
            print(f"expected image Id {image_id} found Id in rados {decoded_name}")
        return False
    return True

def check_image_in_cluster(arg, image_uuid, pool_name, volname_prefix):
    """
    validate pvc information in ceph backend
    """
    image = volname_prefix + image_uuid
    cmd = ['rbd', 'info', image, "--pool", pool_name]
    if not arg.userkey:
        cmd += ["--id", arg.userid, "--key", arg.userkey]
    if arg.toolboxdeployed is True:
        kube = get_cmd_prefix(arg)
        cmd = kube + cmd

    with subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT) as out:
        stdout, stderr = out.communicate()

    if stderr is not None:
        if arg.debug:
            print(b"failed to toolbox %s", stderr)
        return False
    if b"No such file or directory" in stdout:
        if arg.debug:
            print("image not found in cluster")
        return False
    return True

def check_image_uuid_in_rados(arg, image_id, pvc_name, pool_name, is_rbd):
    """
    validate image uuid in rados
    """
    omapkey = f'csi.volume.{image_id}'
    cmd = ['rados', 'getomapval', omapkey, "csi.volname", "--pool", pool_name]
    if not arg.userkey:
        cmd += ["--id", arg.userid, "--key", arg.userkey]
    if not is_rbd:
        cmd += ["--namespace", "csi"]
    if arg.toolboxdeployed is True:
        kube = get_cmd_prefix(arg)
        cmd = kube + cmd

    with subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT) as out:
        stdout, stderr = out.communicate()

    if stderr is not None:
        if arg.debug:
            print("failed to get toolbox %s", stderr)
        return False

    name = b''
    lines = [x.strip() for x in stdout.split(b"\n")]
    for line in lines:
        if b' ' not in line:
            continue
        if b'value' in line and b'bytes' in line:
            continue
        part = re.findall(br'[A-Za-z0-9\-]+', line)
        if part:
            name += part[-1]
    if name.decode() != pvc_name:
        if arg.debug:
            decoded_name = name.decode()
            print(f"expected image Id {pvc_name} found Id in rados {decoded_name}")
        return False
    return True


def get_cmd_prefix(arg):
    """
    Returns command prefix
    """
    kube = [arg.command]
    if arg.kubeconfig != "":
        if arg.command == "oc":
            kube += ["--config", arg.kubeconfig]
        else:
            kube += ["--kubeconfig", arg.kubeconfig]
    tool_box_name = get_tool_box_pod_name(arg)
    kube += ['exec', '-it', tool_box_name, '-n', arg.rooknamespace, '--']
    return kube

def get_image_uuid(volume_handler):
    """
    fetch image uuid from volume handler
    """
    image_id = volume_handler.split('-')
    if len(image_id) < 9:
        return None
    img_id = "-"
    return img_id.join(image_id[len(image_id)-5:])


def get_volume_handler_from_pv(arg, pvname):
    """
    fetch volume handler from pv
    """
    cmd = [arg.command]
    if arg.kubeconfig != "":
        if arg.command == "oc":
            cmd += ["--config", arg.kubeconfig]
        else:
            cmd += ["--kubeconfig", arg.kubeconfig]

    cmd += ['get', 'pv', pvname, '-o', 'json']

    with subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT) as out:
        stdout, stderr = out.communicate()

    if stderr is not None:
        if arg.debug:
            print("failed to pv %s", stderr)
        return ""
    try:
        vol = json.loads(stdout)
        return vol['spec']['csi']['volumeHandle']
    except ValueError as err:
        if arg.debug:
            print("failed to pv %s", err)
    return ""

def get_tool_box_pod_name(arg):
    """
    get tool box pod name
    """
    cmd = [arg.command]
    if arg.kubeconfig != "":
        if arg.command == "oc":
            cmd += ["--config", arg.kubeconfig]
        else:
            cmd += ["--kubeconfig", arg.kubeconfig]
    cmd += ['get', 'po', '-l=app=rook-ceph-tools',
            '-n', arg.rooknamespace, '-o', 'json']

    with subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT) as out:
        stdout, stderr = out.communicate()

    if stderr is not None:
        if arg.debug:
            print("failed to get toolbox pod name %s", stderr)
        return ""
    try:
        pod_name = json.loads(stdout)
        return pod_name['items'][0]['metadata']['name']
    except ValueError as err:
        if arg.debug:
            print("failed to pod %s", err)
    return ""

#pylint: disable=too-many-branches, E0012, W0719
def get_pool_name(arg, vol_id, is_rbd):
    """
    get pool name from ceph backend
    """
    if is_rbd:
        cmd = ['ceph', 'osd', 'lspools', '--format=json']
    else:
        cmd = ['ceph', 'fs', 'ls', '--format=json']
    if  not arg.userkey:
        cmd += ["--id", arg.userid, "--key", arg.userkey]
    if arg.toolboxdeployed is True:
        kube = get_cmd_prefix(arg)
        cmd = kube + cmd

    with subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT) as out:
        stdout, stderr = out.communicate()

    if stderr is not None:
        if arg.debug:
            print("failed to get the pool name %s", stderr)
        return ""
    try:
        pools = json.loads(stdout)
    except ValueError as err:
        if arg.debug:
            print("failed to get the pool name %s", err)
        return ""
    if is_rbd:
        pool_id = vol_id.split('-')
        if len(pool_id) < 4:
            raise Exception("pool id not in the proper format")
        if pool_id[3] in arg.rooknamespace:
            pool_id = pool_id[4]
        else:
            pool_id = pool_id[3]
        for pool in pools:
            if int(pool_id) is int(pool['poolnum']):
                return pool['poolname']
    else:
        for pool in pools:
            return pool['metadata_pool']
    return ""

def check_subvol_in_cluster(arg, subvol_name, fsname):
    """
    Checks if subvolume exists in cluster or not.
    """
    # check if user has specified subvolumeGroup
    subvol_group = get_subvol_group(arg)
    return check_subvol_path(arg, subvol_name, subvol_group, fsname)

def check_subvol_path(arg, subvol_name, subvol_group, fsname):
    """
    Returns True if subvolume path exists in the cluster.
    """
    cmd = ['ceph', 'fs', 'subvolume', 'getpath',
           fsname, subvol_name, subvol_group]
    if not arg.userkey:
        cmd += ["--id", arg.userid, "--key", arg.userkey]
    if arg.toolboxdeployed is True:
        kube = get_cmd_prefix(arg)
        cmd = kube + cmd

    with subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT) as out:
        stdout, stderr = out.communicate()

    if stderr is not None:
        if arg.debug:
            print("failed to get toolbox %s", stderr)
        return False
    if b"Error" in stdout:
        if arg.debug:
            print("subvolume not found in cluster", stdout)
        return False
    return True

def get_subvol_group(arg):
    """
    Returns sub volume group from configmap.
    """
    cmd = [arg.command]
    if arg.kubeconfig != "":
        if arg.command == "oc":
            cmd += ["--config", arg.kubeconfig]
        else:
            cmd += ["--kubeconfig", arg.kubeconfig]
    cmd += ['get', 'cm', arg.configmap, '-o', 'json']
    cmd += ['--namespace', arg.configmapnamespace]

    with subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT) as out:
        stdout, stderr = out.communicate()

    if stderr is not None:
        if arg.debug:
            print("failed to get configmap %s", stderr)
            sys.exit()
    try:
        config_map = json.loads(stdout)
    except ValueError as err:
        print(err, stdout)
        sys.exit()

    # default subvolumeGroup
    subvol_group = "csi"
    cm_data = config_map['data'].get('config.json')
    # Absence of 'config.json' means that the configmap
    # is created by Rook and there won't be any provision to
    # specify subvolumeGroup
    if cm_data:
        if "subvolumeGroup" in cm_data:
            try:
                cm_data_list = json.loads(cm_data)
            except ValueError as err:
                print(err, stdout)
                sys.exit()
            subvol_group = cm_data_list[0]['cephFS']['subvolumeGroup']
    return subvol_group

def is_rbd_pv(arg, pvname, pvdata):
    """
    Checks if volume attributes in a pv has an attribute named 'fsname'.
    If it has, returns False else return True.
    """
    if not pvdata:
        if arg.debug:
            print("failed to get pvdata for %s", pvname)
        sys.exit()

    volume_attr = pvdata['spec']['csi']['volumeAttributes']
    key = 'fsName'
    if key in volume_attr.keys():
        return False
    return True

def get_pv_data(arg, pvname):
    """
    Returns pv data for a given pvname.
    """
    pvdata = {}
    cmd = [arg.command]
    if arg.kubeconfig != "":
        if arg.command == "oc":
            cmd += ["--config", arg.kubeconfig]
        else:
            cmd += ["--kubeconfig", arg.kubeconfig]

    cmd += ['get', 'pv', pvname, '-o', 'json']

    with subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT) as out:
        stdout, stderr = out.communicate()

    if stderr is not None:
        if arg.debug:
            print("failed to get pv %s", stderr)
        sys.exit()
    try:
        pvdata = json.loads(stdout)
    except ValueError as err:
        if arg.debug:
            print("failed to get pv %s", err)
        sys.exit()
    return pvdata

def get_volname_prefix(arg, pvdata):
    """
    Returns volname prefix stored in storage class/pv,
    defaults to "csi-vol-"
    """
    volname_prefix = "csi-vol-"
    if not pvdata:
        if arg.debug:
            print("failed to get pv data")
        sys.exit()
    volume_attr = pvdata['spec']['csi']['volumeAttributes']
    key = 'volumeNamePrefix'
    if key in volume_attr.keys():
        volname_prefix = volume_attr[key]
    return volname_prefix

def get_fsname_from_pvdata(arg, pvdata):
    """
    Returns fsname stored in pv data
    """
    fsname = 'myfs'
    if not pvdata:
        if arg.debug:
            print("failed to get pv data")
        sys.exit()
    volume_attr = pvdata['spec']['csi']['volumeAttributes']
    key = 'fsName'
    if key in volume_attr.keys():
        fsname = volume_attr[key]
    else:
        if arg.debug:
            print("fsname is not set in storageclass/pv")
        sys.exit()
    return fsname

if __name__ == "__main__":
    ARGS = PARSER.parse_args()
    if ARGS.command not in ["kubectl", "oc"]:
        print(f"{ARGS.command} command not supported")
        sys.exit(1)
    if sys.version_info[0] < 3:
        print("python version less than 3 is not supported.")
        sys.exit(1)
    list_pvc_vol_name_mapping(ARGS)
