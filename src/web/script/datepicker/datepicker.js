var picker = {
  // (A) COMMON MONTH NAMES
  months : ["Jan", "Feb", "Mar", "Apr", "May", "Jun",
            "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"],

  // (B) ATTACH DATEPICKER TO TARGET
  //  target: field to populate
  //  container: generate datepicker in here (for inline datepicker)
  //  startmon: start on mon? (optional, default false)
  //  yrange: year select range (optional, default 10)
  //  disableday: days to disable, e.g. [2,7] to disable tue and sun (optional)
  //  onpick : function to call on select date (optional)
  attach : instance => {
    // (B1) SET DEFAULT OPTIONS
    instance.target.readOnly = true; // prevent onscreen keyboard
    instance.startmon = instance.startmon ? true : false;
    instance.yrange = instance.yrange ? instance.yrange : 10;

    // (B2) CURRENT MONTH YEAR (UTC+0)
    var today = new Date(),
        thisMonth = today.getUTCMonth(), // jan is 0
        thisYear = today.getUTCFullYear(); // yyyy

    // (B3) GENERATE HTML
    // (B3-1) DATEPICKER WRAPPER + BASIC STRUCTURE
    instance.hWrap = document.createElement("div");
    instance.hWrap.classList.add("picker-wrap");
    instance.hWrap.innerHTML =
    `<div class="picker">
      <div class="picker-p">
        <div class="picker-b">&lt;</div>
        <select class="picker-m"></select>
        <select class="picker-y"></select>
        <div class="picker-n">&gt;</div>
      </div>
      <div class="picker-d"></div>
    </div>`;
    instance.hMonth = instance.hWrap.querySelector(".picker-m");
    instance.hYear = instance.hWrap.querySelector(".picker-y");
    instance.hDays = instance.hWrap.querySelector(".picker-d");

    // (B3-2) SHIFT PERIOD
    instance.hWrap.querySelector(".picker-b").onclick = () => picker.shift(instance);
    instance.hWrap.querySelector(".picker-n").onclick = () => picker.shift(instance, 1);

    // (B3-3) MONTH SELECTOR
    for (let m in picker.months) {
      let o = document.createElement("option");
      o.value = +m + 1;
      o.text = picker.months[m];
      instance.hMonth.appendChild(o);
    }
    instance.hMonth.selectedIndex = thisMonth;
    instance.hMonth.onchange = () => picker.draw(instance);

    // (B3-4) YEAR SELECTOR
    for (let y = thisYear-instance.yrange; y < thisYear+instance.yrange; y++) {
      let o = document.createElement("option");
      o.value = y;
      o.text = y;
      instance.hYear.appendChild(o);
    }
    instance.hYear.selectedIndex = instance.yrange;
    instance.hYear.onchange = () => picker.draw(instance);

    // (B4) INLINE DATEPICKER - ATTACH INTO CONTAINER
    if (instance.container) { instance.container.appendChild(instance.hWrap); }

    // (B5) POPUP DATEPICKER - ATTACH INTO HTML BODY
    else {
      instance.hWrap.classList.add("popup");
      instance.target.onfocus = () => instance.hWrap.classList.add("show");
      instance.hWrap.onclick = e => { if (e.target == instance.hWrap) { instance.hWrap.classList.remove("show"); }};
      document.body.appendChild(instance.hWrap);
    }

    // (B6) INIT DRAW
    picker.draw(instance);
  },

  // (C) SHIFT PERIOD (BY 1 MONTH)
  shift : (instance, next) => {
    var m = +instance.hMonth.value, y = +instance.hYear.value;
    if (next) {
      m++;
      if (m>12) { m = 1; y++; }
      let max = instance.hYear.querySelector("option:last-child").value;
      if (y>max) { m = 12; y = max; }
    } else {
      m--;
      if (m<1) { m = 12; y--; }
      let min = instance.hYear.querySelector("option:first-child").value;
      if (y<min) { m = 1; y = min; }
    }
    instance.hMonth.value = m;
    instance.hYear.value = y;
    picker.draw(instance);
  },

  // (D) DRAW DAYS IN MONTH
  draw : instance => {
    // (D1) A LOT OF CALCULATIONS
    // (D1-1) SELECTED MONTH YEAR
    var month = instance.hMonth.value,
        year = instance.hYear.value;

    // (D1-2) DATE RANGE CALCULATION (UTC+0)
    var daysInMonth = new Date(Date.UTC(year, month, 0)).getUTCDate(),
        startDay = new Date(Date.UTC(year, month-1, 1)).getUTCDay(), // sun is 0
        endDay = new Date(Date.UTC(year, month-1, daysInMonth)).getUTCDay();
    startDay = startDay==0 ? 7 : startDay,
    endDay = endDay==0 ? 7 : endDay;

    // (D1-3) TODAY (FOR HIGHLIGHTING "TODAY")
    var today = new Date(), todayDate = null;
    if (today.getUTCMonth()+1 == month && today.getUTCFullYear() == year) {
      todayDate = today.getUTCDate();
    }

    // (D1-4) DAY NAMES
    var daynames = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
    if (instance.startmon) { daynames.push("Sun"); }
    else { daynames.unshift("Sun"); }

    // (D2) CALCULATE DATE SQUARES
    // (D2-1) EMPTY SQUARES BEFORE FIRST DAY OF MONTH
    var squares = [];
    if (instance.startmon && startDay!=1) {
      for (let i=1; i<startDay; i++) { squares.push("B"); }
    }
    if (!instance.startmon && startDay!=7) {
      for (let i=0; i<startDay; i++) { squares.push("B"); }
    }

    // (D2-2) DAYS OF MONTH (SOME DAYS DISABLED)
    if (instance.disableday) {
      let thisDay = startDay;
      for (let i=1; i<=daysInMonth; i++) {
        squares.push([i, instance.disableday.includes(thisDay)]);
        thisDay++;
        if (thisDay==8) { thisDay = 1; }
      }
    }

    // (D2-3) DAYS OF MONTH (ALL DAYS ENABLED)
    else {
      for (let i=1; i<=daysInMonth; i++) { squares.push([i, false]);  }
    }

    // (D2-4) EMPTY SQUARES AFTER LAST DAY OF MONTH
    if (instance.startmon && endDay!=7) {
      for (let i=endDay; i<7; i++) { squares.push("B"); }
    }
    if (!instance.startmon && endDay!=6) {
      for (let i=endDay; i<(endDay==7?13:6); i++) { squares.push("B"); }
    }

    // (D3) DRAW HTML FINALLY
    // (D3-1) EMPTY CURRENT
    instance.hDays.innerHTML = "";
    
    // (D3-2) FIRST ROW - DAY NAMES HEADER
    var cell;
    for (let d of daynames) {
      cell = document.createElement("div");
      cell.innerHTML = d;
      cell.classList.add("picker-d-h");
      instance.hDays.appendChild(cell);
    }

    // (D3-3) FOLLOWING ROWS - DATE CELLS
    for (let i=0; i<squares.length; i++) {
      cell = document.createElement("div");
      if (squares[i] == "B") { cell.classList.add("picker-d-b"); }
      else {
        // (D3-2A) CELL DATE
        cell.innerHTML = squares[i][0];

        // (D3-2B) NOT ALLOWED TO CHOOSE THIS DAY
        if (squares[i][1]) { cell.classList.add("picker-d-dd"); }

        // (D3-2C) ALLOWED TO CHOOSE THIS DAY
        else {
          if (squares[i][0] == todayDate) { cell.classList.add("picker-d-td"); }
          cell.classList.add("picker-d-d");
          cell.onclick = () => picker.pick(instance, squares[i][0]);
        }
      }
      instance.hDays.appendChild(cell);
    }
  },

  // (E) CHOOSE A DATE
  pick : (instance, d) => {
    // (E1) GET MONTH YEAR
    let m = instance.hMonth.value,
        y = instance.hYear.value;

    // (E2) FORMAT & SET SELECTED DAY (YYYY-MM-DD)
    if (+m<10) { m = "0" + m; }
    if (+d<10) { d = "0" + d; }
    let formatted = `${y}-${m}-${d}`;
    instance.target.value = formatted;

    // (E3) POPUP ONLY - CLOSE
    if (instance.hWrap.classList.contains("popup")) {
      instance.hWrap.classList.remove("show");
    }

    // (E4) CALL ON PICK IF DEFINED
    if (instance.onpick) { instance.onpick(formatted); }
  }
};