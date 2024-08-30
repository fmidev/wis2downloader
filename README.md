# WIS 2.0 file downloader

Use this container to connect to WIS 2.0 message broker and download bufr files from WIS 2.0 global cache.

Dockerhub image fmidev/wis2downloader updates automatically.

Example command line (see docker-compose)
```
      -server ssl://globalbroker.meteo.fr:8883
      -topic cache/a/wis2/+/data/core/weather/surface-based-observations/synop
      -username everyone
      -password everyone
      -download /downloads
```