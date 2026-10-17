import unittest
from configure import configure
import tomllib


class ProfileTests(unittest.TestCase):
    def test_guest_policy_cannot_be_weakened_by_annotations(self):
        source = '''[hypervisor.dragonball]
shared_fs = "inline-virtio-fs"
virtio_fs_cache = "auto"
virtio_fs_extra_args = ["--thread-pool-size=4", "-o", "writeback,no_open"]
overhead_vcpus = 2.0
overhead_memory = 0
default_memory = 2048
default_maxmemory = 0
image = "/old/image"
guest_hook_path = ""
enable_annotations = ["kernel_params", "default_memory"]
[runtime]
disable_guest_seccomp = true
sandbox_cgroup_only = false
static_sandbox_resource_mgmt = false
hypervisor_name = "dragonball"
'''
        profile = tomllib.loads(configure(source, "/trusted/guest.image"))
        self.assertFalse(profile["runtime"]["disable_guest_seccomp"])
        self.assertEqual(profile["hypervisor"]["dragonball"]["virtio_fs_cache"], "none")
        self.assertEqual(profile["hypervisor"]["dragonball"]["virtio_fs_extra_args"],
                         ["--thread-pool-size=1", "-o", "open,no_writeback_cache"])
        self.assertTrue(profile["runtime"]["sandbox_cgroup_only"])
        self.assertEqual(profile["hypervisor"]["dragonball"]["default_memory"], 2560)
        self.assertEqual(profile["hypervisor"]["dragonball"]["default_maxmemory"], 4096)
        self.assertEqual(profile["hypervisor"]["dragonball"]["enable_annotations"], [])
        self.assertEqual(profile["hypervisor"]["dragonball"]["image"], "/trusted/guest.image")
        self.assertEqual(profile["hypervisor"]["dragonball"]["guest_hook_path"], "/usr/share/codex-dock/hooks")

    def test_unknown_profile_is_rejected(self):
        with self.assertRaises(ValueError):
            configure('[hypervisor.qemu]\nshared_fs="inline-virtio-fs"\n', "/guest.image")


if __name__ == "__main__":
    unittest.main()
