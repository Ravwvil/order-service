document.getElementById('order-form').addEventListener('submit', function(event) {
    event.preventDefault();
    const orderUid = document.getElementById('order-uid').value.trim();
    if (!orderUid) return;

    const orderDetailsDiv = document.getElementById('order-details');
    const errorMessageDiv = document.getElementById('error-message');
    const loader = document.getElementById('loader');
    const jsonOutputDiv = document.getElementById('json-output');
    const jsonToggle = document.getElementById('json-toggle');

    // Reset UI
    orderDetailsDiv.classList.add('hidden');
    jsonOutputDiv.classList.add('hidden');
    errorMessageDiv.classList.add('hidden');
    loader.classList.remove('hidden');
    errorMessageDiv.textContent = '';

    fetch(`/order/${orderUid}`)
        .then(response => {
            if (!response.ok) {
                return response.text().then(text => {
                    let message = `Ошибка: ${response.status}`;
                    if (response.status === 404) {
                        message = 'Заказ не найден.';
                    } else if (text) {
                        try {
                            const errorJson = JSON.parse(text);
                            message = errorJson.error || text;
                        } catch (e) {
                            message = text;
                        }
                    }
                    throw new Error(message);
                });
            }
            return response.json();
        })
        .then(data => {
            loader.classList.add('hidden');
            if (jsonToggle.checked) {
                displayOrderAsJson(data);
            } else {
                displayOrderData(data);
            }
        })
        .catch(error => {
            loader.classList.add('hidden');
            errorMessageDiv.textContent = `Ошибка: ${error.message}`;
            errorMessageDiv.classList.remove('hidden');
        });
});

function displayOrderAsJson(data) {
    const jsonOutputDiv = document.getElementById('json-output');
    const pre = jsonOutputDiv.querySelector('pre');
    pre.textContent = JSON.stringify(data, null, 2);
    jsonOutputDiv.classList.remove('hidden');
}

function displayOrderData(data) {
    const orderDetailsDiv = document.getElementById('order-details');

    // General Info
    const generalInfo = document.getElementById('general-info');
    generalInfo.innerHTML = `
        <p><strong>UID Заказа:</strong> ${data.order_uid}</p>
        <p><strong>Номер отслеживания:</strong> ${data.track_number}</p>
        <p><strong>Покупатель(id):</strong> ${data.customer_id}</p>
        <p><strong>Дата создания:</strong> ${new Date(data.date_created).toLocaleString()}</p>
    `;

    // Delivery Info
    const deliveryInfo = document.getElementById('delivery-info');
    deliveryInfo.innerHTML = `
        <p><strong>Имя:</strong> ${data.delivery.name}</p>
        <p><strong>Телефон:</strong> ${data.delivery.phone}</p>
        <p><strong>Email:</strong> ${data.delivery.email}</p>
        <p><strong>Адрес:</strong> ${data.delivery.city}, ${data.delivery.address}, ${data.delivery.zip}</p>
        <p><strong>Регион:</strong> ${data.delivery.region}</p>
    `;

    // Payment Info
    const paymentInfo = document.getElementById('payment-info');
    paymentInfo.innerHTML = `
        <p><strong>Транзакция:</strong> ${data.payment.transaction}</p>
        <p><strong>Валюта:</strong> ${data.payment.currency}</p>
        <p><strong>Сумма:</strong> ${data.payment.amount} ${data.payment.currency}</p>
        <p><strong>Стоимость доставки:</strong> ${data.payment.delivery_cost} ${data.payment.currency}</p>
        <p><strong>Стоимость товаров:</strong> ${data.payment.goods_total} ${data.payment.currency}</p>
        <p><strong>Банк:</strong> ${data.payment.bank}</p>
        <p><strong>Провайдер:</strong> ${data.payment.provider}</p>
    `;

    // Items table
    const itemsTbody = document.getElementById('items-tbody');
    itemsTbody.innerHTML = ''; // Clear previous items
    data.items.forEach((item, index) => {
        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${index + 1}</td>
            <td>${item.name}</td>
            <td>${item.brand}</td>
            <td>${item.price} ${data.payment.currency}</td>
            <td>${item.sale}%</td>
            <td>${item.total_price} ${data.payment.currency}</td>
        `;
        itemsTbody.appendChild(row);
    });
    
    orderDetailsDiv.classList.remove('hidden');
} 