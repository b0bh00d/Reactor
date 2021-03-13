<p align="center">
  <a href="https://rclone.org/">
    <img width="20%" alt="Reactor" src="https://1.bp.blogspot.com/-M5PLcSana6M/XgBHF7jUjiI/AAAAAAAAUzs/S24qhuijluwKlzIOnc2gntoI-U83ZsrJACLcBGAsYHQ/s1600/rclone_logo.png">
  </a>
</p>

# Reactor
Reactive synchronizing of cloud data using rclone on Windows

## Summary
I've been using rclone for the last couple of years to encrypt and mirror
my Nextcloud data to a secondary service as part of a "disaster recovery"
plan.  The process, however, has been on-demand.  I wrote a number of Python
scripts to take care of the details, but I always needed to run them manually
to get my local data fully synchronized with the backup cloud service.

Not really a big deal, right?  I'm a creature of habit, and I can remember
to do that every morning ... after I check my email ... and after I get through
with this meeting ... and did the builds complete last night? ... and, crap,
I can no longer see straight.  I'll have to do it tomorrow.

This started leaving a big hole in my "disaster" planning when I would extend
the synchronizing window from its already worrying 24 hours to 48, or 72.  I
realilzed that I needed a tool that would use rclone to synchronize my critical
data *whenever it changed*, not just whenever I remembered.

The solution to this is **Reactor**.

## Fire and forget

**Reactor** is a fire-and-forget solution.  Once started, if it has been
configured properly, it will start monitoring one or more locations
(or "watchpoints") on your file system, and will ensure that any cloud storage
(or "remotes" in rclone parlance) associated with those locations are an
exact mirror.  You can configure multiple watchpoints, and each watchpoint
can have multiple rclone "remotes" associated with it.

**Reactor** doesn't use any command-line arguments.  Instead, it is fully
configured using an XML file.  (Hey!  Why not JSON?  Simply put, XML is *much*
more expressive, and it has syntactical support for comments.)  Since I've
developed this to address my specific needs, it is somewhat biased toward
a Windows environment (adjusting it for others should really not require
much work).  Therefore, after seasoning to your liking, the "Reactor.xml"
configruation file should be placed into the %APPDATA% folder so **Reactor**
can find it on startup.

## rclone and Remotes

**Reactor** expects your rclone executable to be visible in your %PATH%
setting such that executing `rclone --version` from a command line
will produce rclone output (and not an error).

It goes without saying that you need to configure your remote cloud
services in rclone before you use **Reactor**.  Additionally, unless
your data footprint is ridiculoulsy small, you should also get your
data image situated on the remote storage *before* you aim **Reactor**
at it.  **Reactor** is really designed to handle *incremental* updates
to existing synchornized data.  Expecting **Reactor** to peform the
initial upload of your tens of gigabytes of data would be a silly
use of the tool, and the gods will weep at your foolishness.  Use
rclone directly to initialize environments.

## Parallelization

It should be noted that **Reactor** sequentializes synchronizations
between watchpoints (one won't start until a previous one completes),
but will parallelize synchronizations among multiple remotes associated
with the same watchpoint.  I tested this with two remotes; the time to
sync took longer on each, but the total time to complete all uploads was
equivalent to running them sequentially (and on good days, sometimes even
ran a few seconds faster).

## winfsnotify

One last thing worth mentioning:  **Reactor** uses a modified version of the
golang/exp/winfsnotify package.  Much to my surprise, the original package
only watched changes in a single folder, ignoring all events of its child
folders and files.  I enhanced winfsnotify to allow it to report on events
in the entire subtree of a watched folder--a critical requirement of **Reactor**.

My changes have been submitted to the project maintainers of winfsnotify for
incorporation into the published package.  If those changes ever make their
way into it, I will remove this locally modified version and update **Reactor**
to use the official version.

## Building

On Windows, compile with: `go generate & go build -ldflags "-H=windowsgui -s -w" .`

For most of the dependencies, the Go system will pull down all modules that
**Reactor** requires as part of the build process.  The one exception is
**goversioninfo**.  You wil need to manually retrieve this package so the first
'generate' step succeeds.  You can do this with

`go get -u github.com/josephspurrier/goversioninfo/cmd/goversioninfo`

before building or installing **Reactor**.

I hope you find this useful.

## Documentation
See the Reactor.xml file for detailed comments concerning configuring
**Reactor**.
