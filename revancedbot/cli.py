import sys
import logging

from revancedbot.app import App

def run_patcher():
    logging.basicConfig(level=logging.DEBUG)
    a = App()
    if len(sys.argv) > 1:
        if sys.argv[1] == 'jobs':
            for item in a.jobs:
                print(item, a.patcher.url_from_job(item) if hasattr(a.patcher, 'url_from_job') else "")
        elif sys.argv[1] == 'fetch':
            print(a.fetched_apks)
        elif sys.argv[1] == 'patch-all':
            print(a.patched_apks)
        else:
            a.patcher(*sys.argv[1:])
    else:
        print("Please provide an argument: jobs, fetch, patch-all, or pass args to patcher.")
