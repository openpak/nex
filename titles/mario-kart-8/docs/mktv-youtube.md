# Mario Kart TV and YouTube

## What Mario Kart 8 (Wii U) does

Findings, with how firm each one is.

- **The console uploads the video to YouTube itself, as the player.** Established by:
  the launch coverage ("upload gameplay directly to YouTube", a Google account is all you
  need; GameSpot, 2014); Nintendo's own Wii U system update 5.3.1 (Dec 2014) fixing the
  highlight-reel upload error 104-1852 (Nintendo Life), which only makes sense if the upload
  is an OS-side function; and the Wii U error-message table in the NintendoClients wiki
  (`Wii-U-Error-Messages`), which carries system-module messages for this flow: 125-0101 and
  125-1010 "Unable to connect to YouTube", 125-1490 "Access to YouTube denied", 125-1730
  "There is no YouTube channel connected to this Google Account. Please create a YouTube
  channel." That last one is what the YouTube Data API returns for a signed-in account without a
  channel, so the console signs the player in to Google (OAuth) and calls YouTube with that
  player's credentials. Clips are capped at 60 seconds.
- **The video is rendered on the console.** The console re-plays the highlight with the game
  engine and encodes it (the same H.264 path the GamePad uses). Nothing in the NEX traffic is a
  video.
- **The server only holds the replay data.** Pretendo's Python server
  (`mario-kart-8-python`) has no YouTube code at all: its DataStore stores a highlight as an
  opaque `mktv/<id>.bin` object plus a thumbnail, and that is the whole server side of Mario
  Kart TV. The data is the replay input the game needs to re-render the race, not pixels.
- **The old Nintendo Mario Kart TV site** (closed 2016-04-05) and Miiverse sharing were the
  other consumers of that replay data; Miiverse is gone since 2017-11. Because the upload
  endpoints and Google's old ClientLogin and Data API v2 are retired, whether the console's
  own YouTube module still works against today's Google is unverified; we have not captured
  that traffic.

Not established (needs a capture from a real Wii U): the exact hosts and protocol of the
console's YouTube module, and the replay format inside `mktv/<id>.bin`.

## Why "game uploads to our server, we render" does not work

Rendering a replay needs the game engine, the game's assets and a Wii U GPU path (Cemu or Quay),
per highlight, with Nintendo's copyrighted data on the server. The replay format is
undocumented. That is not a feasible server-side worker, and OpenPak does not put game assets
on a server. So OpenPak cannot turn the replay data it stores into a video by itself.

## The OpenPak path

The server stores replay data (unchanged). Videos reach YouTube by one of two routes, both feeding
the same worker:

1. **A video file appears in the spool.** Someone with a render (a recording from Quay playing
   the highlight, or a console capture) puts `<data_id>.mp4` in the spool directory. This works
   today.
2. **Redirected console upload (future, needs a capture).** If a capture shows the console's
   YouTube module speaks a protocol we can terminate, a small ingest in front of the worker
   would receive the console's upload (instead of Google) and write the same spool file. Players
   would then upload from the game as before. Not built: the protocol is unknown.

The worker (`youtube.go`, off unless `MK8_YT_ENABLED=1`):

- takes `<data_id>.mp4` from `MK8_YT_SPOOL_DIR` (only that name pattern);
- looks the highlight up in `mk8.datastore_objects` for its owner;
- checks **opt-in**: `mk8.mktv_consent(pid, allowed)`. No row or false means the video is
  deleted and `mk8.mktv_videos.status = 'no_consent'`; nothing is ever uploaded for them. There
  is no in-game way to ask, so the consent flag is to be set from the player's OpenPak account
  settings on the website (not built here);
- uploads with the YouTube Data API v3 resumable protocol (`videos.insert`) to the single
  OpenPak channel, category Gaming, not made for kids, embeddable, privacy `MK8_YT_PRIVACY`
  (default `unlisted`; the title carries no player name);
- records `mk8.mktv_videos(data_id, owner_pid, status, youtube_id, attempts, error)`; the MKTV
  site embeds `https://www.youtube.com/embed/<youtube_id>` for rows with status `uploaded`;
- honours a per-day cap (`MK8_YT_DAILY_UPLOADS`, 1-6) and stops for six hours on a quota refusal
  without counting it as a failure;
- moves handled files to `done/` or `rejected/` under the spool.

The tables are created at startup even while the worker is off, so the site can always join
them. Exposing `youtube_id` in the read API (`/api/v1/mktv`) is a one-line LEFT JOIN on
`mk8.mktv_videos` by `data_id` and is left to the API's owner.

## What the owner must create

1. A Google account for OpenPak and, with it, a **YouTube channel** (sign in to YouTube once and
   create the channel; without it uploads fail with "no channel").
2. A **Google Cloud project** (console.cloud.google.com), signed in as that account.
3. Enable **YouTube Data API v3** (APIs and Services, Library).
4. **OAuth consent screen**: user type External; app name, support email; add scope
   `https://www.googleapis.com/auth/youtube.upload`. Set the publishing status to **In
   production**. In "Testing" status the refresh token expires after 7 days. (An unverified
   production app shows a warning screen only while granting, which is done once.)
5. **Credentials**, OAuth client ID of type **Desktop app** (or Web with a localhost redirect).
   Note the client id and secret.
6. Get a **refresh token** once: open
   `https://accounts.google.com/o/oauth2/v2/auth?client_id=<ID>&redirect_uri=http://127.0.0.1:8085/&response_type=code&scope=https://www.googleapis.com/auth/youtube.upload&access_type=offline&prompt=consent`
   as the channel's account, take the `code` from the redirect, and POST it to
   `https://oauth2.googleapis.com/token` with `grant_type=authorization_code`, the client id,
   secret and the same redirect_uri. Keep the `refresh_token` from the answer.
7. **Quota and audit.** The default quota is 10,000 units a day and `videos.insert` costs 1,600,
   so about 6 uploads a day; ask for more with the YouTube API quota extension form. Since
   2020, uploads from an API project that has not passed YouTube's **API Compliance Audit** are
   locked to private. For public or unlisted embeds on the website, submit the project for that
   audit before relying on it.
8. Put the values in the nex container's environment (shared `.env`), then enable.

## Environment variables

| Variable | Meaning |
| --- | --- |
| `MK8_YT_ENABLED` | `1` to run the worker; anything else leaves it off |
| `MK8_YT_CLIENT_ID` | OAuth client id |
| `MK8_YT_CLIENT_SECRET` | OAuth client secret |
| `MK8_YT_REFRESH_TOKEN` | refresh token from step 6 |
| `MK8_YT_SPOOL_DIR` | directory holding `<data_id>.mp4`; must be writable and on a mounted volume |
| `MK8_YT_PRIVACY` | `public`, `unlisted` (default) or `private` |
| `MK8_YT_DAILY_UPLOADS` | 1-6, default 5 |
| `MK8_YT_SITE_URL` | optional, added to the video description |

With `MK8_YT_ENABLED=1` and anything missing, the worker logs what is missing and stays off.

## If a capture shows route 2 is impossible

If the console's YouTube module cannot be pointed at us (pinned certificates, a protocol that
needs Google-signed tokens), the honest answer is route 1 only: videos are produced outside the
game (Quay recordings, player-submitted clips on the website) and the game's own "Share to
YouTube" button keeps going to Google, if Google still accepts it.
