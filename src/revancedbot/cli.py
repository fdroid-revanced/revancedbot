import logging
import sys

from revancedbot.app import App

def run_patcher():
    logging.basicConfig(level=logging.DEBUG)
    a = App()
    if len(sys.argv) < 2:
        a.patcher()
        return

    if sys.argv[1] == 'jobs':
        for item in a.jobs:
            print(item, getattr(item, "apkpure_url", ""))
    elif sys.argv[1] == 'fetch':
        print(a.fetched_apks)
    elif sys.argv[1] == 'patch-all':
        print(a.patched_apks)
    else:
        a.patcher(*sys.argv[1:])
