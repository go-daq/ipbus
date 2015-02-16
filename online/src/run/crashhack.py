import optparse
import os

parser = optparse.OptionParser()
parser.add_option("-t", "--test", default=False, action="store_true")
parser.add_option("-a", "--allowmod", default=False, action="store_true")
opts, args = parser.parse_args()

tmpdir = "/data/fast/runatbr2/run/"
storage = "/data/fast/runatbr2/storage/"

runcmd = "../../bin/run -threshold 170 -duration 300 -nrun -1 -coincidence -dir %s -store %s" % (tmpdir, storage)
if opts.allowmod:
    runcmd += " -allowmod"
while True:
    print runcmd
    os.system(runcmd)
    files = os.listdir(tmpdir)
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
    if opts.test:
        break
