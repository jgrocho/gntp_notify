# gntp\_notify

gntp\_notify - A bridge between [GNTP][gntp] and [libnotify][libnotify].

## Synopsis

gntp\_notify \[-help\] \[-cachedir \<dir\>\]

## Description

[GNTP][gntp] defines a network protocol for sending desktop notifications
between client programs and a notification daemon.
GNTP notification daemons exist for Mac OS X (as [Growl][growl])
and for Windows (as [Growl for Windows][gfw] and as [Snarl][snarl]).
A GNTP daemon exists for Linux ([Growl for Linux][gfl]),
however, there already exists a notification framework libnotify.

[libnotify][libnotify] defines a notification library for desktop notifications.
A client program uses the libnotify library to send notifications to a daemon.
This way, any program, which complies with the Desktop Notification spec,
can register to receive notifications and display/process them as it sees fit.
However, libnotify does not support notifications sent across the network.

gntp\_notify acts as a bridge between GNTP and libnotify.
That is, when a program sends a notification using GNTP,
gntp\_notify intercepts the notification and forwards it to libnotify.
Thus, any program that can send a notification using GNTP,
can now transparently work with libnotify.

## Options

 -  --help:
    Show usage information.

 -  --cachedir \<dir\>:
    Set the cache directory to the given directory.
    gntp\_notify stores icons on disk.
    By default this is `$XDG_CACHE_HOME/gntp_notify`,
    where `$XDG_CACHE_HOME` defaults to `$HOME/.cache`.

[gntp]: http://www.growlforwindows.com/gfw/help/gntp.aspx "Growl Notification Transport Protocol"
[libnotify]: http://developer.gnome.org/libnotify/ "libnotify"
[growl]: http://growl.info/ "Growl (for Mac OS X)"
[gfw]: http://www.growlforwindows.com/gfw/ "Growl for Windows"
[snarl]: https://sites.google.com/site/snarlapp/ "Snarl"
[gfl]: http://mattn.github.com/growl-for-linux/ "Growl for Linux"
