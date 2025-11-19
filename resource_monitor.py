import psutil, time, os
from datetime import datetime

LOG_PATH = "/home/log/resource.log"
os.makedirs("/home/log", exist_ok=True)

# --- DEĞİŞKENLER BAŞLANGIÇ ---

# MAX Variables (Değer, Zaman)
cpu_1h = cpu_24h = 0
cpu_1h_time = cpu_24h_time = ""

ram_1h = ram_24h = 0
ram_1h_time = ram_24h_time = ""

conn_1h = conn_24h = 0
conn_1h_time = conn_24h_time = ""

pps_in_1h = pps_in_24h = 0
pps_in_1h_time = pps_in_24h_time = ""

drop_1h = drop_24h = 0
drop_1h_time = drop_24h_time = ""

sq_1h = sq_24h = 0
sq_1h_time = sq_24h_time = ""

# Sayaçlar (Önceki değerler)
prev_total = prev_drop = prev_squeeze = None

# Zamanlayıcılar
t_start_1h = time.time()
t_start_24h = time.time()
last_log_time = time.time()

# Anlık CPU "Peak" Buffer (0.1s örnekleme için)
cpu_peak_buffer = 0

# --- FONKSİYONLAR ---

def update_max(current, max_val, max_time, now_str):
    """Eğer yeni değer eskisinden büyükse veya eski 0 ise günceller."""
    if max_val == 0 or current > max_val:
        return current, now_str
    return max_val, max_time

def get_softnet_aggregated():
    """Tüm CPU çekirdeklerindeki ağ istatistiklerini toplar."""
    total = 0
    dropped = 0
    squeeze = 0
    try:
        with open("/proc/net/softnet_stat", "r") as f:
            for line in f:
                cols = line.split()
                total += int(cols[0], 16)
                dropped += int(cols[1], 16)
                squeeze += int(cols[2], 16)
    except:
        pass
    return total, dropped, squeeze

# psutil ısınma turu
psutil.cpu_percent()

# --- ANA DÖNGÜ ---
while True:
    # 1. HIZLI ÖLÇÜM (Her 0.1 saniyede bir CPU'yu kontrol et)
    curr_instant_cpu = psutil.cpu_percent(interval=0)
    if curr_instant_cpu > cpu_peak_buffer:
        cpu_peak_buffer = curr_instant_cpu

    # 2. LOGLAMA VE HESAPLAMA (Her 1.0 saniyede bir çalışır)
    if time.time() - last_log_time >= 1.0:
        now_str = datetime.now().strftime("%H:%M:%S")
        
        # -- Anlık Verileri Al --
        ram = psutil.virtual_memory().percent
        conns = len(psutil.net_connections(kind='inet'))
        
        # Network Delta Hesaplama
        curr_total, curr_drop, curr_sq = get_softnet_aggregated()
        
        if prev_total is None:
            pps_in = 0; pps_drop = 0; pps_sq = 0
        else:
            pps_in = curr_total - prev_total
            pps_drop = curr_drop - prev_drop
            pps_sq = curr_sq - prev_squeeze
        
        # Negatif koruması
        if pps_in < 0: pps_in = 0
        if pps_drop < 0: pps_drop = 0
        if pps_sq < 0: pps_sq = 0

        prev_total = curr_total
        prev_drop = curr_drop
        prev_squeeze = curr_sq

        # -- RESET MANTIĞI (1H) --
        if time.time() - t_start_1h >= 3600:
            cpu_1h = ram_1h = conn_1h = pps_in_1h = drop_1h = sq_1h = 0
            cpu_1h_time = ram_1h_time = conn_1h_time = pps_in_1h_time = drop_1h_time = sq_1h_time = ""
            t_start_1h = time.time()

        # -- RESET MANTIĞI (24H) --
        if time.time() - t_start_24h >= 86400:
            cpu_24h = ram_24h = conn_24h = pps_in_24h = drop_24h = sq_24h = 0
            cpu_24h_time = ram_24h_time = conn_24h_time = pps_in_24h_time = drop_24h_time = sq_24h_time = ""
            t_start_24h = time.time()

        # -- MAX UPDATE (CPU için buffer'daki en yüksek değeri kullanıyoruz!) --
        cpu_1h, cpu_1h_time = update_max(cpu_peak_buffer, cpu_1h, cpu_1h_time, now_str)
        cpu_24h, cpu_24h_time = update_max(cpu_peak_buffer, cpu_24h, cpu_24h_time, now_str)

        ram_1h, ram_1h_time = update_max(ram, ram_1h, ram_1h_time, now_str)
        ram_24h, ram_24h_time = update_max(ram, ram_24h, ram_24h_time, now_str)

        conn_1h, conn_1h_time = update_max(conns, conn_1h, conn_1h_time, now_str)
        conn_24h, conn_24h_time = update_max(conns, conn_24h, conn_24h_time, now_str)

        pps_in_1h, pps_in_1h_time = update_max(pps_in, pps_in_1h, pps_in_1h_time, now_str)
        pps_in_24h, pps_in_24h_time = update_max(pps_in, pps_in_24h, pps_in_24h_time, now_str)

        drop_1h, drop_1h_time = update_max(pps_drop, drop_1h, drop_1h_time, now_str)
        drop_24h, drop_24h_time = update_max(pps_drop, drop_24h, drop_24h_time, now_str)

        sq_1h, sq_1h_time = update_max(pps_sq, sq_1h, sq_1h_time, now_str)
        sq_24h, sq_24h_time = update_max(pps_sq, sq_24h, sq_24h_time, now_str)

        # -- DOSYAYA YAZMA --
        with open(LOG_PATH, "w") as f:
            f.write(f"[{datetime.now()}]\n")
            # Anlık değer olarak o saniyedeki EN YÜKSEK cpu'yu yazıyoruz
            f.write(f"CPU_PEAK: {cpu_peak_buffer:.1f}% | RAM: {ram:.1f}% | CONN: {conns}\n")
            f.write(f"PPS_IN: {pps_in} | DROP: {pps_drop} | SQUEEZE: {pps_sq}\n")
            f.write("-" * 52 + "\n\n")

            # 1H Stats
            f.write(f"CPU_1H_MAX: {cpu_1h:.1f}% ({cpu_1h_time})\n")
            f.write(f"RAM_1H_MAX: {ram_1h:.1f}% ({ram_1h_time})\n")
            f.write(f"CONN_1H_MAX: {conn_1h} ({conn_1h_time})\n\n")
            
            f.write(f"PPS_IN_1H_MAX: {pps_in_1h} ({pps_in_1h_time})\n")
            f.write(f"PPS_DROP_1H_MAX: {drop_1h} ({drop_1h_time})\n")
            f.write(f"SQUEEZE_1H_MAX: {sq_1h} ({sq_1h_time})\n")

            # 24H Stats (EKSİKSİZ)
            f.write("\n--- 24H STATS ---\n")
            f.write(f"CPU_24H_MAX: {cpu_24h:.1f}% ({cpu_24h_time})\n")
            f.write(f"RAM_24H_MAX: {ram_24h:.1f}% ({ram_24h_time})\n")
            f.write(f"CONN_24H_MAX: {conn_24h} ({conn_24h_time})\n\n")
            
            f.write(f"PPS_IN_24H_MAX: {pps_in_24h} ({pps_in_24h_time})\n")
            f.write(f"PPS_DROP_24H_MAX: {drop_24h} ({drop_24h_time})\n")
            f.write(f"SQUEEZE_24H_MAX: {sq_24h} ({sq_24h_time})\n")

        # Değişkenleri sıfırla ve zamanı güncelle
        cpu_peak_buffer = 0
        last_log_time = time.time()

    # CPU'yu yormadan kısa bekleme
    time.sleep(0.1)
