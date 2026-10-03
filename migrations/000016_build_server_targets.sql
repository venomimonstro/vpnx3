BEGIN;

ALTER TABLE build_jobs
  DROP CONSTRAINT IF EXISTS build_jobs_target_check;

ALTER TABLE build_jobs
  ADD CONSTRAINT build_jobs_target_check CHECK (
    target IN (
      'android_apk','android_aab','chrome_zip','firefox_zip','ios_ipa',
      'controlplane_linux_amd64','node_agent_linux_amd64','vpn_worker_linux_amd64',
      'probe_agent_linux_amd64','ingress_proxy_linux_amd64'
    )
  );

COMMIT;
