import os

tmpdir = "/data/fast/runatbr2/run/"
storage = "/data/fast/runatbr2/stoarge/"

runcmd ="../../bin/run -threshold 150 -duration 300 -nrun -1 -coincidence -dir %s -store %s" % (tmpdir, storage)
while True:
    os.system(runcmd)
    files = os.listidr(tmpdir)
    for fn in files:
        tmpfn = os.path.join(tmpdir, fn)
        stat = os.stat(tmpfn)
        size = stat.st_size * 1e-6 # MB
        if size > 20:
            storefn = os.path.join(storage, fn)
            cmd = "mv %s %s" % (tmpfn, storefn)
            os.system(cmd)
        else:
            cmd = "rm %s" % tmpfn
            os.system(cmd)
