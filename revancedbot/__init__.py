import logging
import sys

from .app import App

def run_patcher():
    logging.basicConfig(level=logging.DEBUG)
    a = App()
    if len(sys.argv) < 2:
        print("Usage: repatcher <command> [args...]")
        return

    if sys.argv[1] == 'jobs':
        for item in a.jobs:
            # We assume apkpure_url was an oversight in the original since it doesn't exist on PatchJob.
            # Printing item is fine.
            print(item)
    elif sys.argv[1] == 'fetch':
        print(a.fetched_apks)
    elif sys.argv[1] == 'patch-all':
        print(a.patched_apks)
    else:
        a.patcher(*sys.argv[1:])
