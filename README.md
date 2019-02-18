# Panopticon

A web app providing a UI for managing images uploaded from cameras, such as security cameras or landscape cameras. The server accepts uploaded photos from cameras, categorizes them (such as motion-detection, "collected" periodic uploads, etc.) and stores them. These photos then become viewable in a web UI.

Images are stored to and retrieved from the filesystem, including metadata (i.e. timestamps come from file timestamps, and camera association comes from directory tree.) A sqlite database contains top-level settings and metadata (i.e. list of known cameras and users.)

Currently functional, but something of a work in progress.

# Features

Below is a rough feature list.

## Authentication via Google-OAuth2

* Support for multiple users
* Distinction between "privileged" and unprivileged users determining whether a user can see private-flagged cameras; that is, provides a way to grant access to non-sensitive cameras (such as landscape cameras) to certain users

## Multiple Cameras

* Support for "diurnal" (daylight-only) cameras, by simply ignoring uploads received after civil sunset at camera's location; useful for landscape cameras

## Display Latest Image
* Refresh 5s during daylight
* Alert button
* Status indicator (night, etc.)

## Image saving

* Images can be pinned (saved), meaning they get copied to a directory not subject to periodic purging.
* This provides a workflow where users can review a day's images, save ones that are interesting, and leave the rest to be purged per schedule

## Timelapses
* Construct a timelapse from all photos for a given day spaced 30s apart
* Folder of these by day

## Cleanup Thread
* Purge non-pinned images after midnight of day taken
* Purge all non-pinned media after 3 weeks

## Motion endpoint
* Scripts on camera push images upon motion
* Scripts on camera push videos upon motion

## Admin
* Add email
* QR setup

## Sqlite
* Users
  * Email
  * Name
  * Privileged (bool)
* Cameras
  * Name string
  * ID string
  * Address string
  * Diurnal bool
  * Timelapse enum
    * None
    * Daylight
    * Motion
  * Image pull URL
  * RTSP pull URL
  * Latitude & Longitude
  * Dewarp (bool) - whether to apply a dewarp (fisheye distortion correction) transformation to uploaded images
  * Private

## [LATER] Display current video
* Pull RTSP from camera on-demand
* Reflect media to clients

## [LATER] Geofences

## [LATER] Image classifier

# Links

[Wyze camera alt firmware project](https://github.com/EliasKotlyar/Xiaomi-Dafang-Hacks/tree/master/firmware_mod/scripts)