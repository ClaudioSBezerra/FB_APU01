@echo off
echo Starting FB_APU01 with .env.FB_APU01...
docker compose --env-file .env.FB_APU01 up -d --build
echo.
echo Check status with: docker compose --env-file .env.FB_APU01 ps
pause