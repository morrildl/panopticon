# Dacha Neopanopticon

Some code for managing cameras at home and dacha.

# Features

## Authentication via Google-OAuth2
## Multiple Cameras

## Display Latest Image
* Refresh 5s during daylight
* Alert button
* Status indicator (night, etc.)

## Image History
* Alerted images
* Images resulting from motion

## Timelapsen
* Record every 6th image (i.e. every 30s)
* After sunset, generate timelapse
* Folder of these by day

## Cleanup Thread
* Purge non-pinned (timer) images after midnight of day taken
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
* Sites
  * Name
  * ID
  * Lat
  * Lon
* Images
  * SHA256 (filename)
  * Timestamp
  * Camera ID
  * Motivation enum
    * Timer
    * Motion
    * Flagged
* Videos
  * SHA256 (filename)
  * Timestamp
  * Camera ID
  * Motivation enum
    * Daily Timelapse
    * Motion
    * [LATER] Flagged
* Timelapsen
  * SHA256 (filename)
  * Timestamp
  * Camera ID
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

## [LATER] Display current video
* Pull RTSP from camera on-demand
* Reflect media to clients

## [LATER] Geofences

## [LATER] Image classifier
